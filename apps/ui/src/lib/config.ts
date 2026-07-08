// Hardcoded for now (no runtime config injection yet). These match apps/broadcast's
// dev defaults: PORT=4433, ROUTE=/, DEV_CERT_HASH_PORT=8080.
export const WEBTRANSPORT_URL = 'https://localhost:4433/'
export const CERT_HASH_URL = 'http://localhost:8080/cert-hash'

// Video codec, resolution, framerate, and bitrate are per-share choices now —
// see lib/video-presets.ts (the sharer picks a preset in Settings; the receiver
// learns the codec from the stream). Only the keyframe cadence is fixed here.
export const KEYFRAME_INTERVAL_MS = 2000

// Audio is captured with the screen (getDisplayMedia audio) and sent as Opus.
// The encoder is configured from the real capture format and each audio
// datagram is prefixed with its sample rate + channel count, so the decoder
// matches whatever the source actually is (mono/stereo, 44.1/48 kHz).
export const AUDIO_CODEC = 'opus'
export const AUDIO_BITRATE = 128_000
