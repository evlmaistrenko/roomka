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
  const codecLength = payload[0]
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

export function fragment(
  meta: FrameMeta,
  payload: Uint8Array,
  maxDatagramSize: number,
): Uint8Array[] {
  const maxPayload = Math.max(1, maxDatagramSize - HEADER_SIZE)
  const chunkCount = Math.max(1, Math.ceil(payload.byteLength / maxPayload))
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

// Reassembler collects datagram fragments per (senderId, frameId) and emits a
// complete frame once all its chunks arrive. Incomplete frames older than a
// small window are dropped (a lost fragment means a dropped frame).
export class Reassembler {
  private pending = new Map<string, Pending>()

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
    const meta: FrameMeta = { senderId, frameId, timestamp, keyframe, audio }

    // Audio and video have independent frameId sequences, so the reassembly key
    // must include the kind to avoid collisions.
    const key = `${senderId}:${audio ? 'a' : 'v'}:${frameId}`
    let entry = this.pending.get(key)
    if (!entry) {
      entry = { chunks: new Array(chunkCount), received: 0, meta }
      this.pending.set(key, entry)
    }
    if (entry.chunks[chunkIndex]) return null // duplicate fragment
    entry.chunks[chunkIndex] = datagram.slice(HEADER_SIZE)
    entry.received++
    if (entry.received < entry.chunks.length) return null

    this.pending.delete(key)
    this.dropStale(senderId, frameId, audio)

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

  private dropStale(senderId: number, frameId: number, audio: boolean) {
    const threshold = frameId - 30
    const kind = audio ? 'a' : 'v'
    for (const key of this.pending.keys()) {
      const [sid, k, fid] = key.split(':')
      if (Number(sid) === senderId && k === kind && Number(fid) < threshold) {
        this.pending.delete(key)
      }
    }
  }
}
