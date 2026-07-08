// Datagram wire format for fragmenting encoded video frames across QUIC
// datagrams (which cap at ~1200 bytes). Each datagram carries a fixed header
// followed by a slice of one EncodedVideoChunk.
//
// Header (little-endian, 21 bytes):
//   u32 senderId | u32 frameId | f64 timestamp | u16 chunkIndex | u16 chunkCount | u8 flags
export const HEADER_SIZE = 21
const FLAG_KEYFRAME = 1
const FLAG_AUDIO = 2

// Audio payloads are prefixed with sampleRate (u32) + channels (u8) so the
// receiver's Opus decoder matches the source format.
export const AUDIO_CONFIG_PREFIX = 5

// Video keyframe payloads are prefixed with the codec string so the receiver can
// configure each sender's decoder from the stream itself — senders may use
// different codecs (H.264 / VP8 / VP9). Layout: [u8 codecLength][utf-8 codec]
// [encoded chunk bytes]. Delta frames carry no prefix (the decoder is already
// configured by the time a delta arrives).
const codecTextEncoder = new TextEncoder()
const codecTextDecoder = new TextDecoder()

export function withCodec(codec: string, chunk: Uint8Array): Uint8Array {
  const codecBytes = codecTextEncoder.encode(codec)
  const out = new Uint8Array(1 + codecBytes.byteLength + chunk.byteLength)
  out[0] = codecBytes.byteLength
  out.set(codecBytes, 1)
  out.set(chunk, 1 + codecBytes.byteLength)
  return out
}

export function readCodec(payload: Uint8Array): {
  codec: string
  chunk: Uint8Array
} {
  // Guard against a malformed keyframe: an empty payload (payload[0] would be
  // undefined) or a codecLength that runs past the buffer would otherwise yield
  // an empty/garbage codec and throw deep inside decoder.configure. Throw here
  // so the receiver's per-datagram catch drops just this frame.
  if (payload.byteLength < 1) {
    throw new Error('keyframe payload missing codec prefix')
  }
  const codecLength = payload[0]
  if (1 + codecLength > payload.byteLength) {
    throw new Error('keyframe codec length exceeds payload')
  }
  const codec = codecTextDecoder.decode(payload.subarray(1, 1 + codecLength))
  return { codec, chunk: payload.subarray(1 + codecLength) }
}

export type FrameMeta = {
  senderId: number
  frameId: number
  timestamp: number
  keyframe: boolean
  audio: boolean
}

// AES-GCM additional authenticated data binding the ciphertext to its
// frame-level routing metadata. The relay is untrusted, so without this it
// could relabel a peer's still-valid ciphertext with a different senderId
// (identity spoof) or flip the audio flag (feeding garbage into the wrong
// decoder). We bind only the frame-invariant fields — senderId, frameId,
// timestamp, flags — not chunkIndex/chunkCount, which legitimately differ
// across the fragments of one encrypted frame. Layout (little-endian, 17 bytes):
//   u32 senderId | u32 frameId | f64 timestamp | u8 flags
export const AAD_SIZE = 17

export function frameAad(meta: FrameMeta): Uint8Array {
  const aad = new Uint8Array(AAD_SIZE)
  const view = new DataView(aad.buffer)
  view.setUint32(0, meta.senderId, true)
  view.setUint32(4, meta.frameId, true)
  view.setFloat64(8, meta.timestamp, true)
  view.setUint8(
    16,
    (meta.keyframe ? FLAG_KEYFRAME : 0) | (meta.audio ? FLAG_AUDIO : 0),
  )
  return aad
}

export function fragment(
  meta: FrameMeta,
  payload: Uint8Array,
  maxDatagramSize: number,
): Uint8Array[] {
  const maxPayload = Math.max(1, maxDatagramSize - HEADER_SIZE)
  const chunkCount = Math.max(1, Math.ceil(payload.byteLength / maxPayload))
  // chunkCount rides in a u16; fail loudly rather than silently truncating it.
  // Unreachable for real encoded frames (this is a ~77 MB frame), but cheap.
  if (chunkCount > 0xffff) {
    throw new Error(`frame too large to fragment: ${chunkCount} chunks`)
  }
  const datagrams: Uint8Array[] = []
  for (let i = 0; i < chunkCount; i++) {
    const slice = payload.subarray(i * maxPayload, (i + 1) * maxPayload)
    const buf = new Uint8Array(HEADER_SIZE + slice.byteLength)
    const view = new DataView(buf.buffer)
    view.setUint32(0, meta.senderId, true)
    view.setUint32(4, meta.frameId, true)
    view.setFloat64(8, meta.timestamp, true)
    view.setUint16(16, i, true)
    view.setUint16(18, chunkCount, true)
    view.setUint8(
      20,
      (meta.keyframe ? FLAG_KEYFRAME : 0) | (meta.audio ? FLAG_AUDIO : 0),
    )
    buf.set(slice, HEADER_SIZE)
    datagrams.push(buf)
  }
  return datagrams
}

export type ReassembledFrame = FrameMeta & { data: Uint8Array }

type Pending = {
  chunks: (Uint8Array | undefined)[]
  received: number
  meta: FrameMeta
}

// How many frames behind the newest a partial frame may fall before it's
// abandoned (a lost fragment means a dropped frame).
const STALE_FRAME_WINDOW = 30
// Hard cap on in-flight partial frames across all streams — a memory backstop
// against a flood of never-completing frames (e.g. spoofed senderIds/frameIds
// from a malicious relay). Oldest partials are evicted past this.
const MAX_PENDING_FRAMES = 256
// Upper bound on a frame's fragment count. maxDatagram is ~1200 bytes, so this
// caps a single frame near ~5 MB — far above any real encoded frame, while
// stopping a spoofed chunkCount (up to 65535) from pre-allocating a huge array.
const MAX_CHUNKS = 4096

// Reassembler collects datagram fragments per (senderId, kind, frameId) and
// emits a complete frame once all its chunks arrive. Partial frames the stream
// has moved well past are pruned on every datagram — not only on completion —
// so sustained loss can't grow memory without bound.
export class Reassembler {
  private pending = new Map<string, Pending>()
  // Highest frameId seen per (senderId, kind), the anchor for staleness pruning.
  private latest = new Map<string, number>()

  push(datagram: Uint8Array): ReassembledFrame | null {
    const view = new DataView(
      datagram.buffer,
      datagram.byteOffset,
      datagram.byteLength,
    )
    const senderId = view.getUint32(0, true)
    const frameId = view.getUint32(4, true)
    const timestamp = view.getFloat64(8, true)
    const chunkIndex = view.getUint16(16, true)
    const chunkCount = view.getUint16(18, true)
    const flags = view.getUint8(20)
    const keyframe = (flags & FLAG_KEYFRAME) !== 0
    const audio = (flags & FLAG_AUDIO) !== 0

    // Reject nonsensical fragmentation from a corrupt or malicious datagram: an
    // out-of-range chunkIndex would leave a frame that can never complete (and
    // grow a sparse array), and an oversized chunkCount pre-allocates a large
    // array per spoofed frame.
    if (chunkCount < 1 || chunkCount > MAX_CHUNKS || chunkIndex >= chunkCount) {
      return null
    }

    const meta: FrameMeta = { senderId, frameId, timestamp, keyframe, audio }

    // Audio and video have independent frameId sequences, so the reassembly key
    // must include the kind to avoid collisions.
    const streamKey = `${senderId}:${audio ? 'a' : 'v'}`
    const newest = Math.max(this.latest.get(streamKey) ?? frameId, frameId)
    this.latest.set(streamKey, newest)
    this.dropStale(streamKey, newest)

    const key = `${streamKey}:${frameId}`
    let entry = this.pending.get(key)
    if (!entry) {
      entry = { chunks: new Array(chunkCount), received: 0, meta }
      this.pending.set(key, entry)
      this.enforceCap()
    } else if (entry.chunks.length !== chunkCount) {
      return null // a later fragment disagrees on the frame's chunk count
    }
    if (entry.chunks[chunkIndex]) return null // duplicate fragment
    entry.chunks[chunkIndex] = datagram.slice(HEADER_SIZE)
    entry.received++
    if (entry.received < entry.chunks.length) return null

    this.pending.delete(key)

    let total = 0
    for (const chunk of entry.chunks) total += chunk!.byteLength
    const data = new Uint8Array(total)
    let offset = 0
    for (const chunk of entry.chunks) {
      data.set(chunk!, offset)
      offset += chunk!.byteLength
    }
    return { ...meta, data }
  }

  // Drop partials of a stream that have fallen more than STALE_FRAME_WINDOW
  // frames behind its newest, anchored on the high-water-mark frameId so a
  // stream that never completes a frame still gets pruned.
  private dropStale(streamKey: string, newest: number) {
    const threshold = newest - STALE_FRAME_WINDOW
    for (const key of this.pending.keys()) {
      const lastColon = key.lastIndexOf(':')
      if (key.slice(0, lastColon) !== streamKey) continue
      if (Number(key.slice(lastColon + 1)) < threshold) this.pending.delete(key)
    }
  }

  // Evict the oldest-inserted partials once the global cap is exceeded (Map
  // preserves insertion order, so keys().next() is the oldest).
  private enforceCap() {
    while (this.pending.size > MAX_PENDING_FRAMES) {
      const oldest = this.pending.keys().next().value
      if (oldest === undefined) break
      this.pending.delete(oldest)
    }
  }
}
