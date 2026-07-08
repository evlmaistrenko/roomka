import { AUDIO_CODEC } from './config'
import { decrypt, getKey } from './e2ee'
import { AUDIO_CONFIG_PREFIX, readCodec, Reassembler } from './protocol'

export type VideoFrameHandler = (senderId: number, frame: VideoFrame) => void
export type AudioDataHandler = (senderId: number, data: AudioData) => void

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
): () => void {
  const reader = transport.datagrams.readable.getReader()
  const reassembler = new Reassembler()
  const videoDecoders = new Map<number, VideoDecoder>()
  const audioDecoders = new Map<number, AudioDecoder>()
  // Codec each sender's video decoder is currently configured for (from the
  // codec tag on that sender's keyframes). Absent = not yet configured.
  const videoCodecs = new Map<number, string>()

  const videoDecoderFor = (senderId: number): VideoDecoder => {
    let decoder = videoDecoders.get(senderId)
    if (!decoder) {
      decoder = new VideoDecoder({
        output: (frame) => onVideoFrame(senderId, frame),
        error: (error) => console.error('video decoder error', error),
      })
      videoDecoders.set(senderId, decoder)
    }
    return decoder
  }

  // Configured lazily from the sample rate + channels prefixed on the first
  // audio packet, so the decoder matches the source format.
  const audioDecoderFor = (
    senderId: number,
    sampleRate: number,
    channels: number,
  ): AudioDecoder => {
    let decoder = audioDecoders.get(senderId)
    if (!decoder) {
      decoder = new AudioDecoder({
        output: (data) => onAudioData(senderId, data),
        error: (error) => console.error('audio decoder error', error),
      })
      decoder.configure({
        codec: AUDIO_CODEC,
        sampleRate,
        numberOfChannels: channels,
      })
      audioDecoders.set(senderId, decoder)
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

      const frame = reassembler.push(value)
      if (!frame) continue

      // The payload is E2EE ciphertext; decrypt before decoding. Sustained
      // auth failures on complete frames signal a wrong key — report it once.
      const data = await decrypt(key, frame.data)
      if (!data) {
        if (++decryptFailures === DECRYPT_FAILURE_LIMIT) onDecryptFailure()
        continue
      }
      decryptFailures = 0

      if (frame.audio) {
        const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
        const sampleRate = view.getUint32(0, true)
        const channels = view.getUint8(4)
        audioDecoderFor(frame.senderId, sampleRate, channels).decode(
          new EncodedAudioChunk({
            type: 'key',
            timestamp: frame.timestamp,
            data: data.subarray(AUDIO_CONFIG_PREFIX),
          }),
        )
        continue
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
        }
      } else if (!videoCodecs.has(frame.senderId)) {
        continue // no keyframe seen yet — can't decode deltas
      }
      videoDecoderFor(frame.senderId).decode(
        new EncodedVideoChunk({
          type: frame.keyframe ? 'key' : 'delta',
          timestamp: frame.timestamp,
          data: chunk,
        }),
      )
    }
  }
  void pump().catch((error: unknown) => console.error('receiver stopped', error))

  return () => {
    stopped = true
    void reader.cancel().catch(() => {})
    for (const decoder of videoDecoders.values()) {
      if (decoder.state !== 'closed') decoder.close()
    }
    for (const decoder of audioDecoders.values()) {
      if (decoder.state !== 'closed') decoder.close()
    }
  }
}
