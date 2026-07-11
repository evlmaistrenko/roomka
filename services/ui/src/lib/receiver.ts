import { AUDIO_CODEC } from "./config"
import { decrypt, getKey } from "./e2ee"
import {
	AUDIO_CONFIG_PREFIX,
	Reassembler,
	frameAad,
	readCodec,
} from "./protocol"

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
	framesDecoded: number
	framesDropped: number
	decryptFailures: number
	decoderErrors: number
	decodeQueueSize: number
	audioSampleRate: number
	audioChannels: number
}

type Counters = Omit<ReceiverStats, "senderId" | "decodeQueueSize"> & {
	lastFrameId: number // highest video frameId seen, for gap-based drop counting
}

export type Receiver = {
	stop: () => void
	getStats: () => ReceiverStats[]
}

// Consecutive fully-reassembled frames that fail AES-GCM auth before we conclude
// the E2EE key is wrong. Lost fragments are dropped earlier (at reassembly), so
// auth failures on *complete* frames almost always mean a key mismatch, not loss.
const DECRYPT_FAILURE_LIMIT = 15

// startReceiving reads datagrams, reassembles them into encoded frames, and
// decodes each sender's video and audio streams with their own decoders. Video
// decoding for a sender only begins once its first keyframe arrives. If
// decryption keeps failing, onDecryptFailure fires once (likely wrong E2EE key).
export function startReceiving(
	transport: WebTransport,
	onVideoFrame: VideoFrameHandler,
	onAudioData: AudioDataHandler,
	onDecryptFailure: () => void,
): Receiver {
	const reader = transport.datagrams.readable.getReader()
	const reassembler = new Reassembler()
	const videoDecoders = new Map<number, VideoDecoder>()
	const audioDecoders = new Map<number, AudioDecoder>()

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
				framesDecoded: 0,
				framesDropped: 0,
				decryptFailures: 0,
				decoderErrors: 0,
				audioSampleRate: 0,
				audioChannels: 0,
				lastFrameId: -1,
			}
			stats.set(senderId, s)
		}
		return s
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

	let stopped = false
	let decryptFailures = 0
	const pump = async () => {
		const key = await getKey()
		while (!stopped) {
			const { value, done } = await reader.read()
			if (done) break
			if (!value) continue

			// Isolate every datagram: a malformed frame (a too-short datagram, a
			// decoder that throws InvalidStateError after going closed, a bad audio
			// format) must drop only that frame, never tear down reception from every
			// sender. The read() itself stays outside — its rejection ends the loop.
			try {
				// Count bytes/datagrams per sender for the debug overlay (senderId is
				// the first header field). Guarded so a runt datagram just isn't counted.
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
				if (!frame) continue

				// The payload is E2EE ciphertext; decrypt before decoding. The frame's
				// metadata is bound as AAD, so a broadcast server that tampered with
				// the header fails auth here. Sustained auth failures on complete
				// frames signal a wrong key — report it once.
				const data = await decrypt(key, frame.data, frameAad(frame))
				if (!data) {
					statsFor(frame.senderId).decryptFailures++
					if (++decryptFailures === DECRYPT_FAILURE_LIMIT) onDecryptFailure()
					continue
				}
				decryptFailures = 0

				if (frame.audio) {
					const view = new DataView(
						data.buffer,
						data.byteOffset,
						data.byteLength,
					)
					const sampleRate = view.getUint32(0, true)
					const channels = view.getUint8(4)
					const s = statsFor(frame.senderId)
					s.audioSampleRate = sampleRate
					s.audioChannels = channels
					audioDecoderFor(frame.senderId, sampleRate, channels).decode(
						new EncodedAudioChunk({
							type: "key",
							timestamp: frame.timestamp,
							data: data.subarray(AUDIO_CONFIG_PREFIX),
						}),
					)
					continue
				}

				// Count dropped video frames from gaps in the frameId sequence (a
				// never-reassembled frame leaves a hole). Only on forward progress;
				// reordering is ignored and a big backward jump is treated as a restart.
				const s = statsFor(frame.senderId)
				if (s.lastFrameId < 0) {
					s.lastFrameId = frame.frameId
				} else if (frame.frameId > s.lastFrameId) {
					const gap = frame.frameId - s.lastFrameId
					if (gap > 1 && gap < 300) s.framesDropped += gap - 1
					s.lastFrameId = frame.frameId
				} else if (frame.frameId < s.lastFrameId - 100) {
					s.lastFrameId = frame.frameId
				}

				// Keyframes carry a codec tag; configure (or reconfigure, if the sender
				// switched codecs) the decoder from it, then decode the stripped chunk.
				// Deltas are only decodable once a keyframe has configured the decoder.
				let chunk = data
				if (frame.keyframe) {
					const tagged = readCodec(data)
					chunk = tagged.chunk
					if (videoCodecs.get(frame.senderId) !== tagged.codec) {
						videoDecoderFor(frame.senderId).configure({ codec: tagged.codec })
						videoCodecs.set(frame.senderId, tagged.codec)
						s.codec = tagged.codec
					}
				} else if (!videoCodecs.has(frame.senderId)) {
					continue // no keyframe seen yet — can't decode deltas
				}
				videoDecoderFor(frame.senderId).decode(
					new EncodedVideoChunk({
						type: frame.keyframe ? "key" : "delta",
						timestamp: frame.timestamp,
						data: chunk,
					}),
				)
			} catch (error) {
				console.error("dropped datagram", error)
			}
		}
	}
	void pump().catch((error: unknown) =>
		console.error("receiver stopped", error),
	)

	const getStats = (): ReceiverStats[] =>
		Array.from(stats.entries()).map(([senderId, s]) => ({
			senderId,
			codec: s.codec,
			width: s.width,
			height: s.height,
			bytes: s.bytes,
			datagrams: s.datagrams,
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
		void reader.cancel().catch(() => {})
		for (const decoder of videoDecoders.values()) {
			if (decoder.state !== "closed") decoder.close()
		}
		for (const decoder of audioDecoders.values()) {
			if (decoder.state !== "closed") decoder.close()
		}
	}

	return { stop, getStats }
}
