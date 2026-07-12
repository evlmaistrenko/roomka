import { AUDIO_CODEC } from "./config"
import { decrypt, getKey } from "./e2ee"
import {
	AUDIO_CONFIG_PREFIX,
	type FrameMeta,
	Reassembler,
	type ReorderFrame,
	VideoReorderBuffer,
	frameAad,
	readCodec,
	unpackKeyframeMessage,
} from "./protocol"

// Upper bound on a single keyframe stream. A real keyframe is well under a
// megabyte even at 4K; this caps memory against a misbehaving or malicious relay
// sending an unbounded stream. An oversized stream is dropped (the next keyframe
// recovers).
const MAX_KEYFRAME_STREAM_BYTES = 4 * 1024 * 1024

// Cap on keyframe streams read concurrently. Keyframes are seconds apart, so a
// couple are ever in flight legitimately; this bounds buffered memory
// (≤ this × MAX_KEYFRAME_STREAM_BYTES) against a relay that opens many slow
// streams at once. Beyond it, new incoming streams are dropped.
const MAX_CONCURRENT_KEYFRAME_STREAMS = 8

export type VideoFrameHandler = (senderId: number, frame: VideoFrame) => void
export type AudioDataHandler = (senderId: number, data: AudioData) => void

// Per-sender receive stats for the debug overlay. All counters are cumulative
// except decodeQueueSize (instantaneous); the consumer derives bitrate/fps from
// deltas of `bytes`/`framesDecoded`.
export type ReceiverStats = {
	senderId: number
	codec: string
	width: number
	height: number
	bytes: number
	datagrams: number
	keyframeStreams: number // keyframes received over reliable streams
	framesDecoded: number
	framesDropped: number
	decryptFailures: number
	decoderErrors: number
	decodeQueueSize: number
	audioSampleRate: number
	audioChannels: number
}

type Counters = Omit<ReceiverStats, "senderId" | "decodeQueueSize">

export type Receiver = {
	stop: () => void
	getStats: () => ReceiverStats[]
}

// Consecutive fully-reassembled frames that fail AES-GCM auth before we conclude
// the E2EE key is wrong. Lost fragments are dropped earlier (at reassembly), so
// auth failures on *complete* frames almost always mean a key mismatch, not loss.
const DECRYPT_FAILURE_LIMIT = 15

// startReceiving reads both datagrams and reliable streams, turns them back into
// encoded frames, and decodes each sender's video and audio with its own decoder.
// A sender may send its keyframes over datagrams or over streams; the receiver
// handles both simultaneously (a stream-carried keyframe is reordered ahead of
// any deltas that overtook it — see VideoReorderBuffer). Video decoding for a
// sender only begins once its first keyframe arrives. If decryption keeps
// failing, onDecryptFailure fires once (likely wrong E2EE key).
export function startReceiving(
	transport: WebTransport,
	onVideoFrame: VideoFrameHandler,
	onAudioData: AudioDataHandler,
	onDecryptFailure: () => void,
): Receiver {
	const datagramReader = transport.datagrams.readable.getReader()
	const streamReader = (
		transport.incomingUnidirectionalStreams as ReadableStream<
			ReadableStream<Uint8Array>
		>
	).getReader()
	const reassembler = new Reassembler()
	const videoDecoders = new Map<number, VideoDecoder>()
	const audioDecoders = new Map<number, AudioDecoder>()
	// Per-sender ordering of video frames before decode: keyframes (possibly from a
	// stream) are reordered ahead of the datagram deltas that raced past them.
	const videoReorder = new Map<number, VideoReorderBuffer>()

	const stats = new Map<number, Counters>()
	const statsFor = (senderId: number): Counters => {
		let s = stats.get(senderId)
		if (!s) {
			s = {
				codec: "",
				width: 0,
				height: 0,
				bytes: 0,
				datagrams: 0,
				keyframeStreams: 0,
				framesDecoded: 0,
				framesDropped: 0,
				decryptFailures: 0,
				decoderErrors: 0,
				audioSampleRate: 0,
				audioChannels: 0,
			}
			stats.set(senderId, s)
		}
		return s
	}

	const reorderFor = (senderId: number): VideoReorderBuffer => {
		let buffer = videoReorder.get(senderId)
		if (!buffer) {
			buffer = new VideoReorderBuffer()
			videoReorder.set(senderId, buffer)
		}
		return buffer
	}
	// Codec each sender's video decoder is currently configured for (from the
	// codec tag on that sender's keyframes). Absent = not yet configured.
	const videoCodecs = new Map<number, string>()
	// `${sampleRate}:${channels}` each sender's audio decoder is configured for,
	// so a mid-stream format change triggers a reconfigure rather than decoding
	// against the stale first-packet format.
	const audioConfigs = new Map<number, string>()

	const videoDecoderFor = (senderId: number): VideoDecoder => {
		let decoder = videoDecoders.get(senderId)
		if (!decoder) {
			decoder = new VideoDecoder({
				output: (frame) => {
					const s = statsFor(senderId)
					s.framesDecoded++
					s.width = frame.displayWidth
					s.height = frame.displayHeight
					onVideoFrame(senderId, frame)
				},
				// A fatal decoder error (corrupt bitstream, unsupported frame) leaves
				// the decoder closed. Drop it so this sender gets a fresh decoder on its
				// next keyframe, instead of wedging playback for everyone. Clearing the
				// codec entry forces reconfiguration and makes deltas wait for a key.
				error: (error) => {
					console.error("video decoder error", error)
					statsFor(senderId).decoderErrors++
					videoDecoders.delete(senderId)
					videoCodecs.delete(senderId)
				},
			})
			videoDecoders.set(senderId, decoder)
		}
		return decoder
	}

	// Configured from the sample rate + channels prefixed on each audio packet,
	// so the decoder matches the source format — and reconfigured if that format
	// changes mid-stream.
	const audioDecoderFor = (
		senderId: number,
		sampleRate: number,
		channels: number,
	): AudioDecoder => {
		let decoder = audioDecoders.get(senderId)
		if (!decoder) {
			decoder = new AudioDecoder({
				output: (data) => onAudioData(senderId, data),
				// Drop a failed decoder so the next audio packet (each carries its own
				// format prefix) recreates and reconfigures it, rather than killing the
				// receive loop.
				error: (error) => {
					console.error("audio decoder error", error)
					statsFor(senderId).decoderErrors++
					audioDecoders.delete(senderId)
					audioConfigs.delete(senderId)
				},
			})
			audioDecoders.set(senderId, decoder)
		}
		const config = `${sampleRate}:${channels}`
		if (audioConfigs.get(senderId) !== config) {
			decoder.configure({
				codec: AUDIO_CODEC,
				sampleRate,
				numberOfChannels: channels,
			})
			audioConfigs.set(senderId, config)
		}
		return decoder
	}

	// Decode one in-order video frame. A keyframe carries a codec tag that
	// configures (or reconfigures, if the sender switched codecs) the decoder; a
	// delta is only decodable once a keyframe has done so. Isolated so a single bad
	// frame (e.g. a malformed codec tag) drops just itself.
	const decodeVideo = (senderId: number, frame: ReorderFrame) => {
		try {
			let chunk = frame.data
			if (frame.keyframe) {
				const tagged = readCodec(frame.data)
				chunk = tagged.chunk
				if (videoCodecs.get(senderId) !== tagged.codec) {
					videoDecoderFor(senderId).configure({ codec: tagged.codec })
					videoCodecs.set(senderId, tagged.codec)
					statsFor(senderId).codec = tagged.codec
				}
			} else if (!videoCodecs.has(senderId)) {
				return // no keyframe seen yet (or decoder reset) — can't decode deltas
			}
			videoDecoderFor(senderId).decode(
				new EncodedVideoChunk({
					type: frame.keyframe ? "key" : "delta",
					timestamp: frame.timestamp,
					data: chunk,
				}),
			)
		} catch (error) {
			console.error("dropped video frame", error)
		}
	}

	let stopped = false
	let decryptFailures = 0

	// Shared handler for a frame from either transport (a reassembled datagram
	// frame or a whole keyframe stream). The payload is E2EE ciphertext bound to its
	// metadata as AAD, so a relay that tampered with the header fails auth here;
	// sustained failures on complete frames signal a wrong key. Video is released to
	// the decoder in frameId order (a stream keyframe may arrive after its deltas).
	const processFrame = async (
		key: CryptoKey,
		meta: FrameMeta,
		ciphertext: Uint8Array,
	) => {
		const data = await decrypt(key, ciphertext, frameAad(meta))
		if (!data) {
			statsFor(meta.senderId).decryptFailures++
			if (++decryptFailures === DECRYPT_FAILURE_LIMIT) onDecryptFailure()
			return
		}
		decryptFailures = 0

		if (meta.audio) {
			const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
			const sampleRate = view.getUint32(0, true)
			const channels = view.getUint8(4)
			const s = statsFor(meta.senderId)
			s.audioSampleRate = sampleRate
			s.audioChannels = channels
			audioDecoderFor(meta.senderId, sampleRate, channels).decode(
				new EncodedAudioChunk({
					type: "key",
					timestamp: meta.timestamp,
					data: data.subarray(AUDIO_CONFIG_PREFIX),
				}),
			)
			return
		}

		// Frames the reorder buffer skips over — a lost delta, or a keyframe too slow
		// to wait for — count as drops.
		const buffer = reorderFor(meta.senderId)
		const skippedBefore = buffer.skipped
		const released = buffer.push(
			{
				frameId: meta.frameId,
				gopId: meta.gopId,
				timestamp: meta.timestamp,
				keyframe: meta.keyframe,
				data,
			},
			performance.now(),
		)
		statsFor(meta.senderId).framesDropped += buffer.skipped - skippedBefore
		for (const frame of released) decodeVideo(meta.senderId, frame)
	}

	const pumpDatagrams = async () => {
		const key = await getKey()
		while (!stopped) {
			const { value, done } = await datagramReader.read()
			if (done) break
			if (!value) continue

			// Isolate every datagram: a malformed frame must drop only itself, never
			// tear down reception. The read() itself stays outside — its rejection ends
			// the loop.
			try {
				// Count bytes/datagrams per sender for the overlay (senderId is the first
				// header field). Guarded so a runt datagram just isn't counted.
				if (value.byteLength >= 4) {
					const senderId = new DataView(
						value.buffer,
						value.byteOffset,
						value.byteLength,
					).getUint32(0, true)
					const s = statsFor(senderId)
					s.datagrams++
					s.bytes += value.byteLength
				}

				const frame = reassembler.push(value)
				if (frame) await processFrame(key, frame, frame.data)
			} catch (error) {
				console.error("dropped datagram", error)
			}
		}
	}

	// Read each keyframe stream to completion, then hand it to the shared path. The
	// whole stream is one keyframe: [frameAad header][ciphertext].
	const readKeyframeStream = async (
		key: CryptoKey,
		stream: ReadableStream<Uint8Array>,
	) => {
		const message = await readStreamBounded(stream, MAX_KEYFRAME_STREAM_BYTES)
		if (!message) return // empty or oversized — dropped
		const { meta, ciphertext } = unpackKeyframeMessage(message)
		const s = statsFor(meta.senderId)
		s.keyframeStreams++
		s.bytes += message.byteLength
		await processFrame(key, meta, ciphertext)
	}

	let activeStreamReads = 0
	const pumpStreams = async () => {
		const key = await getKey()
		while (!stopped) {
			const { value, done } = await streamReader.read()
			if (done) break
			if (!value) continue
			// Bound concurrent reads: past the cap, drop (cancel) the stream rather than
			// buffer it — a well-behaved sender never has this many keyframes in flight.
			if (activeStreamReads >= MAX_CONCURRENT_KEYFRAME_STREAMS) {
				void value.cancel().catch(() => {})
				continue
			}
			// Read each stream independently so a slow one doesn't hold up the next; a
			// rejection drops just that keyframe.
			activeStreamReads++
			void readKeyframeStream(key, value)
				.catch((error: unknown) =>
					console.error("dropped keyframe stream", error),
				)
				.finally(() => {
					activeStreamReads--
				})
		}
	}

	void pumpDatagrams().catch((error: unknown) =>
		console.error("receiver stopped", error),
	)
	void pumpStreams().catch((error: unknown) =>
		console.error("receiver stream reader stopped", error),
	)

	const getStats = (): ReceiverStats[] =>
		Array.from(stats.entries()).map(([senderId, s]) => ({
			senderId,
			codec: s.codec,
			width: s.width,
			height: s.height,
			bytes: s.bytes,
			datagrams: s.datagrams,
			keyframeStreams: s.keyframeStreams,
			framesDecoded: s.framesDecoded,
			framesDropped: s.framesDropped,
			decryptFailures: s.decryptFailures,
			decoderErrors: s.decoderErrors,
			decodeQueueSize: videoDecoders.get(senderId)?.decodeQueueSize ?? 0,
			audioSampleRate: s.audioSampleRate,
			audioChannels: s.audioChannels,
		}))

	const stop = () => {
		stopped = true
		void datagramReader.cancel().catch(() => {})
		void streamReader.cancel().catch(() => {})
		for (const decoder of videoDecoders.values()) {
			if (decoder.state !== "closed") decoder.close()
		}
		for (const decoder of audioDecoders.values()) {
			if (decoder.state !== "closed") decoder.close()
		}
	}

	return { stop, getStats }
}

// Read a whole unidirectional stream into one buffer, returning null if it's
// empty or exceeds max (in which case the read is cancelled — the stream is
// abandoned). Reads incrementally so an oversized stream is cut off early rather
// than fully buffered.
async function readStreamBounded(
	stream: ReadableStream<Uint8Array>,
	max: number,
): Promise<Uint8Array | null> {
	const reader = stream.getReader()
	const chunks: Uint8Array[] = []
	let total = 0
	try {
		for (;;) {
			const { value, done } = await reader.read()
			if (done) break
			if (!value) continue
			total += value.byteLength
			if (total > max) {
				await reader.cancel().catch(() => {})
				return null
			}
			chunks.push(value)
		}
	} finally {
		try {
			reader.releaseLock()
		} catch {
			// already released (e.g. after cancel) — nothing to do
		}
	}
	if (total === 0) return null
	if (chunks.length === 1) return chunks[0]
	const out = new Uint8Array(total)
	let offset = 0
	for (const chunk of chunks) {
		out.set(chunk, offset)
		offset += chunk.byteLength
	}
	return out
}
