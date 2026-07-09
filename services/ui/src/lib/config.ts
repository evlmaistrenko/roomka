// Broadcast connection parameters come from the shared ROOMKA_PUBLIC_BROADCAST_*
// env vars (Vite exposes them via envPrefix), so host/port/route are defined
// once in the monorepo .env and read by both the relay and this client. Falls
// back to the dev defaults so the app still runs without a configured .env.
const host = import.meta.env.ROOMKA_PUBLIC_BROADCAST_HOST ?? "localhost"
const port = import.meta.env.ROOMKA_PUBLIC_BROADCAST_PORT ?? "4433"
const route = import.meta.env.ROOMKA_PUBLIC_BROADCAST_ROUTE ?? "/"
const certHashPort =
	import.meta.env.ROOMKA_PUBLIC_BROADCAST_CERT_HASH_PORT ?? "8080"

export const WEBTRANSPORT_URL = `https://${host}:${port}${route}`
export const CERT_HASH_URL = `http://${host}:${certHashPort}/cert-hash`

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
export const AUDIO_CODEC = "opus"
export const AUDIO_BITRATE = 128_000
