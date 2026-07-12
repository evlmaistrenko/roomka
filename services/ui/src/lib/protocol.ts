// Datagram wire format for fragmenting encoded video frames across QUIC
// datagrams (which cap at ~1200 bytes). Each datagram carries a fixed header
// followed by a slice of one EncodedVideoChunk.
//
// Header (little-endian, 25 bytes):
//   u32 senderId | u32 frameId | f64 timestamp | u16 chunkIndex | u16 chunkCount
//   | u8 flags | u32 gopId
export const HEADER_SIZE = 25
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
		throw new Error("keyframe payload missing codec prefix")
	}
	const codecLength = payload[0]
	if (1 + codecLength > payload.byteLength) {
		throw new Error("keyframe codec length exceeds payload")
	}
	const codec = codecTextDecoder.decode(payload.subarray(1, 1 + codecLength))
	return { codec, chunk: payload.subarray(1 + codecLength) }
}

export type FrameMeta = {
	senderId: number
	frameId: number
	// frameId of the keyframe that starts this frame's group-of-pictures (a
	// keyframe's own gopId equals its frameId; audio, independently decodable, sets
	// it to its frameId too). Lets the receiver's reorder buffer tell a lost delta
	// of an already-decoded GOP (skip it) from a delta still waiting on its
	// reliably-arriving keyframe (hold for it) — see VideoReorderBuffer.
	gopId: number
	timestamp: number
	keyframe: boolean
	audio: boolean
}

// AES-GCM additional authenticated data binding the ciphertext to its
// frame-level routing metadata. The broadcast server is untrusted, so without
// this it could relabel a peer's still-valid ciphertext with a different
// senderId (identity spoof) or flip the audio flag (feeding garbage into the
// wrong decoder). We bind only the frame-invariant fields — senderId, frameId,
// timestamp, flags, gopId — not chunkIndex/chunkCount, which legitimately differ
// across the fragments of one encrypted frame. Layout (little-endian, 21 bytes):
//   u32 senderId | u32 frameId | f64 timestamp | u8 flags | u32 gopId
export const AAD_SIZE = 21

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
	view.setUint32(17, meta.gopId, true)
	return aad
}

// Inverse of frameAad: recover a frame's routing metadata from its 17-byte
// header. The keyframe-stream path (see packKeyframeMessage) puts this header on
// the wire verbatim and reuses it as the AES-GCM additional data, so parsing it
// back yields metadata guaranteed byte-identical to what encrypt authenticated.
export function parseFrameAad(header: Uint8Array): FrameMeta {
	if (header.byteLength < AAD_SIZE) {
		throw new Error("frame header too short")
	}
	const view = new DataView(header.buffer, header.byteOffset, header.byteLength)
	const flags = view.getUint8(16)
	return {
		senderId: view.getUint32(0, true),
		frameId: view.getUint32(4, true),
		timestamp: view.getFloat64(8, true),
		keyframe: (flags & FLAG_KEYFRAME) !== 0,
		audio: (flags & FLAG_AUDIO) !== 0,
		gopId: view.getUint32(17, true),
	}
}

// A keyframe carried over a reliable WebTransport stream instead of fragmented
// across unreliable datagrams. One stream carries exactly one keyframe as
// [17-byte frameAad header][E2EE ciphertext]: the header doubles as routing
// metadata and as the ciphertext's additional data, so a relay that tampers with
// it fails authentication exactly as on the datagram path. Reliable delivery
// means a keyframe (and thus the GOP that depends on it) is never lost to a
// single dropped fragment — the receiver reorders it back ahead of any deltas
// that overtook it (see VideoReorderBuffer).
export function packKeyframeMessage(
	meta: FrameMeta,
	ciphertext: Uint8Array,
): Uint8Array {
	const header = frameAad(meta)
	const out = new Uint8Array(header.byteLength + ciphertext.byteLength)
	out.set(header, 0)
	out.set(ciphertext, header.byteLength)
	return out
}

export function unpackKeyframeMessage(message: Uint8Array): {
	meta: FrameMeta
	ciphertext: Uint8Array
} {
	// parseFrameAad validates the length; the ciphertext is everything after it.
	return {
		meta: parseFrameAad(message),
		ciphertext: message.subarray(AAD_SIZE),
	}
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
		view.setUint32(21, meta.gopId, true)
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

// How many frames behind the current frame a partial may fall before it's
// abandoned (a lost fragment means a dropped frame).
const STALE_FRAME_WINDOW = 30
// Hard cap on in-flight partial frames across all streams — a memory backstop
// against a flood of never-completing frames (e.g. spoofed senderIds/frameIds
// from a malicious broadcast server). Oldest partials are evicted past this.
const MAX_PENDING_FRAMES = 256
// Upper bound on a frame's fragment count. maxDatagram is ~1200 bytes, so this
// caps a single frame near ~5 MB — far above any real encoded frame, while
// stopping a spoofed chunkCount (up to 65535) from pre-allocating a huge array.
const MAX_CHUNKS = 4096

// Reassembler collects datagram fragments per (senderId, kind, frameId) and
// emits a complete frame once all its chunks arrive. Partial frames older than
// the current datagram's frame (by more than a window) are pruned on every
// datagram — not only on completion — so sustained loss can't grow memory
// without bound.
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
		const gopId = view.getUint32(21, true)
		const keyframe = (flags & FLAG_KEYFRAME) !== 0
		const audio = (flags & FLAG_AUDIO) !== 0

		// Reject nonsensical fragmentation from a corrupt or malicious datagram: an
		// out-of-range chunkIndex would leave a frame that can never complete (and
		// grow a sparse array), and an oversized chunkCount pre-allocates a large
		// array per spoofed frame.
		if (chunkCount < 1 || chunkCount > MAX_CHUNKS || chunkIndex >= chunkCount) {
			return null
		}

		const meta: FrameMeta = {
			senderId,
			frameId,
			gopId,
			timestamp,
			keyframe,
			audio,
		}

		// Audio and video have independent frameId sequences, so the reassembly key
		// must include the kind to avoid collisions.
		const streamKey = `${senderId}:${audio ? "a" : "v"}`
		// Prune relative to the *current* datagram's frameId rather than a
		// persistent high-water mark: a stream that restarts at frameId 0, or a
		// single forged out-of-range frameId, must not permanently evict every
		// subsequent legitimate frame. A late-arriving old fragment just prunes
		// less that tick (its frameId is low); in-order progress prunes stragglers.
		this.dropStale(streamKey, frameId)

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
	// frames behind `anchor` (the current datagram's frameId), so a stream that
	// never completes a frame still gets pruned as it progresses.
	private dropStale(streamKey: string, anchor: number) {
		const threshold = anchor - STALE_FRAME_WINDOW
		for (const key of this.pending.keys()) {
			const lastColon = key.lastIndexOf(":")
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

export type ReorderFrame = {
	frameId: number
	gopId: number // frameId of the keyframe starting this frame's GOP (see FrameMeta)
	timestamp: number
	keyframe: boolean
	data: Uint8Array // decrypted; a keyframe still carries its withCodec prefix
}

// Longest the reorder buffer waits for a lost delta of an already-decoded GOP
// before skipping the gap. It only applies to lost deltas — a gap that is (or is
// waiting behind) a not-yet-decoded keyframe is held indefinitely instead, since
// keyframes arrive reliably over a stream and will re-sync the decode.
const REORDER_MAX_WAIT_MS = 250
// Memory backstop: never hold more than this many out-of-order frames per sender.
// Also bounds the wait for a keyframe that never arrives (dropped by the relay
// under load) — past this the buffer force-skips and the next keyframe re-syncs.
const REORDER_MAX_FRAMES = 256
// A keyframe this far below the cursor is a fresh share (the sender restarted and
// reset its frame counter), not a stale duplicate — re-sync to it rather than
// ignore it.
const REORDER_RESTART_GAP = 256

// VideoReorderBuffer releases a sender's video frames to its decoder in frameId
// order. It's needed because keyframes may travel over a reliable stream while
// deltas travel over datagrams: a small delta routinely overtakes the larger
// keyframe it depends on, and decoding that delta first corrupts the picture. The
// buffer holds early deltas until their keyframe lands, treats every keyframe as
// an unconditional re-sync point (a keyframe decodes standalone), and — using
// each frame's gopId — skips only genuinely lost deltas while waiting out an
// in-flight (reliable) keyframe. On an in-order, lossless datagram-only stream it
// is a pass-through (each frame drains immediately).
export class VideoReorderBuffer {
	// The frameId to release next; null until the first keyframe bootstraps it
	// (deltas can't be decoded before a keyframe configures the decoder anyway).
	private next: number | null = null
	// gopId of the most recently released keyframe: buffered frames with a gopId at
	// or below this belong to a GOP we've already decoded (so a gap before them is a
	// lost delta, safe to skip); a higher gopId means their keyframe hasn't arrived.
	private syncedGopId = -1
	private pending = new Map<number, ReorderFrame>()
	private stallSince: number | null = null
	// Cumulative frames skipped over and never delivered — surfaced as a drop stat.
	skipped = 0

	push(frame: ReorderFrame, now: number): ReorderFrame[] {
		const released: ReorderFrame[] = []

		if (frame.keyframe) {
			const restart =
				this.next !== null && frame.frameId < this.next - REORDER_RESTART_GAP
			if (this.next === null || frame.frameId >= this.next || restart) {
				// A keyframe decodes on its own, so it re-syncs the stream. On a restart
				// the whole buffer is pre-restart garbage; otherwise discard only the
				// frames this keyframe supersedes. Either way the discarded frames are
				// drops. Then release it, advance the cursor, and drain waiters.
				if (restart) {
					this.skipped += this.pending.size
					this.pending.clear()
				} else {
					for (const id of this.pending.keys()) {
						if (id <= frame.frameId) {
							this.pending.delete(id)
							this.skipped++
						}
					}
				}
				released.push(frame)
				this.next = frame.frameId + 1
				this.syncedGopId = frame.frameId
				this.stallSince = null
				this.drain(released)
			}
			// else: a stale/duplicate keyframe below the cursor — ignore it.
		} else if (this.next !== null && frame.frameId >= this.next) {
			// A delta we haven't passed: buffer it (ignoring duplicates), then release
			// whatever is now contiguous.
			if (!this.pending.has(frame.frameId))
				this.pending.set(frame.frameId, frame)
			this.drain(released)
		}
		// A delta before the first keyframe, or older than the cursor, is dropped.

		this.skipStalledHead(released, now)
		return released
	}

	// Release buffered frames while they continue the sequence without a gap.
	private drain(released: ReorderFrame[]) {
		while (this.next !== null) {
			const frame = this.pending.get(this.next)
			if (!frame) break
			this.pending.delete(this.next)
			released.push(frame)
			this.next++
			this.stallSince = null
		}
	}

	// When the head of line is missing while later frames pile up, decide whether
	// to skip the gap. The smallest buffered frame tells us what the gap is: if it
	// belongs to a GOP we've already decoded (gopId <= syncedGopId), the missing
	// frame is a lost delta — skip after a short reorder-tolerance budget. If it
	// belongs to a not-yet-decoded GOP (gopId > syncedGopId), its keyframe is still
	// in flight and arrives reliably, so hold indefinitely and let that keyframe
	// re-sync us — capped only by the memory backstop, in case the keyframe was
	// dropped by the relay under load.
	private skipStalledHead(released: ReorderFrame[], now: number) {
		if (
			this.next === null ||
			this.pending.size === 0 ||
			this.pending.has(this.next)
		) {
			this.stallSince = null
			return
		}

		let smallestId = Infinity
		let smallestGopId = 0
		for (const [id, frame] of this.pending) {
			if (id < smallestId) {
				smallestId = id
				smallestGopId = frame.gopId
			}
		}
		if (smallestId === Infinity) return

		const overCapacity = this.pending.size > REORDER_MAX_FRAMES
		if (smallestGopId > this.syncedGopId) {
			// Waiting on an in-flight reliable keyframe — hold unless memory forces it.
			if (!overCapacity) {
				this.stallSince = null
				return
			}
		} else {
			// A lost delta of a decoded GOP — skip once the wait budget elapses.
			if (this.stallSince === null) this.stallSince = now
			if (now - this.stallSince <= REORDER_MAX_WAIT_MS && !overCapacity) return
		}

		this.skipped += smallestId - this.next
		this.next = smallestId
		this.stallSince = null
		this.drain(released)
	}
}
