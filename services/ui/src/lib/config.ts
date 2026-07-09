// Hardcoded for now (no runtime config injection yet). These match apps/broadcast's
// dev defaults: PORT=4433, ROUTE=/, DEV_CERT_HASH_PORT=8080.
export const WEBTRANSPORT_URL = 'https://localhost:4433/'
export const CERT_HASH_URL = 'http://localhost:8080/cert-hash'

// Video codec, resolution, framerate, and bitrate are per-share choices now —
// see lib/video-presets.ts (the sharer picks a preset in Settings; the receiver
// learns the codec from the stream). Only the keyframe cadence is fixed here.
export const KEYFRAME_INTERVAL_MS = 2000

// Conservative upper bound on datagram payload size. The relay bridges two
// independent QUIC connections (sharer↔relay and relay↔viewer), each with its
// own path MTU, and forwards one datagram verbatim to every viewer — it cannot
// re-fragment — so a datagram must fit the SMALLEST viewer's path, which the
// sharer can't observe. QUIC guarantees any connected peer accepts a 1200-byte
// UDP payload (~1150 usable after packet overhead), so we cap safely below that;
// the sharer additionally clamps to its own live maxDatagramSize.
export const SAFE_MAX_DATAGRAM_SIZE = 1100

// Audio is captured with the screen (getDisplayMedia audio) and sent as Opus.
// The encoder is configured from the real capture format and each audio
// datagram is prefixed with its sample rate + channel count, so the decoder
// matches whatever the source actually is (mono/stereo, 44.1/48 kHz).
export const AUDIO_CODEC = 'opus'
export const AUDIO_BITRATE = 128_000
