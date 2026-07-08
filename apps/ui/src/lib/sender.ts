import { AUDIO_BITRATE, AUDIO_CODEC, KEYFRAME_INTERVAL_MS } from './config'
import { encrypt, getKey } from './e2ee'
import {
  AUDIO_CONFIG_PREFIX,
  fragment,
  withCodec,
  type FrameMeta,
} from './protocol'
import { buildEncoderConfig, type VideoPreset } from './video-presets'

export type ScreenShare = {
  stream: MediaStream
  stop: () => void
}

// startShare captures the screen (and its audio, if shared), encodes video with
// WebCodecs using the chosen preset's codec and audio with Opus, encrypts each
// chunk (E2EE — the relay only sees ciphertext), and emits datagram fragments
// through `send`. The video encoder is configured from each frame's real
// dimensions so aspect ratio is never distorted (only the capture height is
// constrained), and it forces a keyframe every KEYFRAME_INTERVAL_MS.
export async function startShare(
  send: (datagram: Uint8Array) => void,
  senderId: number,
  maxDatagramSize: number,
  preset: VideoPreset,
): Promise<ScreenShare> {
  const stream = await navigator.mediaDevices.getDisplayMedia({
    video: {
      height: { ideal: preset.height },
      frameRate: { ideal: preset.framerate },
    },
    audio: true,
  })
  const key = await getKey()

  let stopped = false
  const cleanups: Array<() => void> = []
  const isStopped = () => stopped

  startVideo(stream, send, senderId, maxDatagramSize, key, preset, cleanups, isStopped)
  startAudio(stream, send, senderId, maxDatagramSize, key, cleanups, isStopped)

  const stop = () => {
    if (stopped) return
    stopped = true
    for (const cleanup of cleanups) cleanup()
    for (const track of stream.getTracks()) track.stop()
  }

  // The user can also end the share from the browser's own UI.
  stream.getVideoTracks()[0]?.addEventListener('ended', stop)

  return { stream, stop }
}

// encryptAndSend serializes encryption per track (a promise chain) so chunks are
// sent in encode order despite crypto being async.
function makeSender(
  key: CryptoKey,
  send: (datagram: Uint8Array) => void,
  maxDatagramSize: number,
) {
  let chain: Promise<void> = Promise.resolve()
  return (meta: FrameMeta, plaintext: Uint8Array) => {
    chain = chain
      .then(async () => {
        const payload = await encrypt(key, plaintext)
        for (const datagram of fragment(meta, payload, maxDatagramSize)) {
          send(datagram)
        }
      })
      .catch((error: unknown) => console.error('encrypt/send failed', error))
  }
}

function startVideo(
  stream: MediaStream,
  send: (datagram: Uint8Array) => void,
  senderId: number,
  maxDatagramSize: number,
  key: CryptoKey,
  preset: VideoPreset,
  cleanups: Array<() => void>,
  isStopped: () => boolean,
) {
  const track = stream.getVideoTracks()[0]
  if (!track) return

  const emit = makeSender(key, send, maxDatagramSize)
  let frameId = 0
  const encoder = new VideoEncoder({
    output: (chunk) => {
      const bytes = new Uint8Array(chunk.byteLength)
      chunk.copyTo(bytes)
      // Tag keyframes with the codec so the receiver can configure its decoder
      // from the stream (senders may use different codecs); deltas go raw.
      const keyframe = chunk.type === 'key'
      emit(
        {
          senderId,
          frameId: frameId++,
          timestamp: chunk.timestamp,
          keyframe,
          audio: false,
        },
        keyframe ? withCodec(preset.codec, bytes) : bytes,
      )
    },
    error: (error) => console.error('video encoder error', error),
  })

  const reader = new MediaStreamTrackProcessor({ track }).readable.getReader()
  cleanups.push(() => {
    void reader.cancel().catch(() => {})
    if (encoder.state !== 'closed') encoder.close()
  })

  let configuredWidth = 0
  let configuredHeight = 0
  let lastKeyframeMs = 0

  const pump = async () => {
    while (!isStopped()) {
      const { value: frame, done } = await reader.read()
      if (done || !frame) break

      // Round to even dimensions (VP8/H.264) and (re)configure the encoder to
      // the frame's real size to avoid rescaling/distortion.
      const width = frame.displayWidth & ~1
      const height = frame.displayHeight & ~1
      if (width > 0 && (width !== configuredWidth || height !== configuredHeight)) {
        encoder.configure(buildEncoderConfig(preset, width, height))
        configuredWidth = width
        configuredHeight = height
        lastKeyframeMs = 0
      }

      if (encoder.state === 'configured') {
        const nowMs = frame.timestamp / 1000
        const keyFrame = nowMs - lastKeyframeMs >= KEYFRAME_INTERVAL_MS
        if (keyFrame) lastKeyframeMs = nowMs
        encoder.encode(frame, { keyFrame })
      }
      frame.close()
    }
  }
  void pump()
}

function startAudio(
  stream: MediaStream,
  send: (datagram: Uint8Array) => void,
  senderId: number,
  maxDatagramSize: number,
  key: CryptoKey,
  cleanups: Array<() => void>,
  isStopped: () => boolean,
) {
  const track = stream.getAudioTracks()[0]
  if (!track) {
    console.warn('no audio shared (enable "Share audio" in the picker)')
    return
  }

  const emit = makeSender(key, send, maxDatagramSize)
  let frameId = 0
  let sampleRate = 0
  let channels = 0
  const encoder = new AudioEncoder({
    output: (chunk) => {
      // Prefix each Opus packet with its sample rate (u32) + channel count (u8)
      // so the receiver can configure its decoder to match (encrypted together).
      const payload = new Uint8Array(AUDIO_CONFIG_PREFIX + chunk.byteLength)
      const view = new DataView(payload.buffer)
      view.setUint32(0, sampleRate, true)
      view.setUint8(4, channels)
      const opus = new Uint8Array(chunk.byteLength)
      chunk.copyTo(opus)
      payload.set(opus, AUDIO_CONFIG_PREFIX)

      emit(
        {
          senderId,
          frameId: frameId++,
          timestamp: chunk.timestamp,
          keyframe: true, // every Opus frame is independently decodable
          audio: true,
        },
        payload,
      )
    },
    error: (error) => console.error('audio encoder error', error),
  })

  const reader = new MediaStreamTrackProcessor<AudioData>({
    track,
  }).readable.getReader()
  cleanups.push(() => {
    void reader.cancel().catch(() => {})
    if (encoder.state !== 'closed') encoder.close()
  })

  const pump = async () => {
    while (!isStopped()) {
      const { value: data, done } = await reader.read()
      if (done || !data) break
      // Configure from the real capture format on the first packet.
      if (encoder.state === 'unconfigured') {
        sampleRate = data.sampleRate
        channels = data.numberOfChannels
        encoder.configure({
          codec: AUDIO_CODEC,
          sampleRate,
          numberOfChannels: channels,
          bitrate: AUDIO_BITRATE,
        })
      }
      if (encoder.state === 'configured') encoder.encode(data)
      data.close()
    }
  }
  void pump()
}
