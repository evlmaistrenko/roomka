// Broadcast connection config. In production the container injects
// window.__ROOMKA_CONFIG__ (from /config.js) so one built image serves any host;
// in development the UI reads the same values from the shared root .env via Vite
// (see vite.config.ts). There are no defaults — a missing value is a hard error.
const runtime = window.__ROOMKA_CONFIG__ ?? {}

function required(name: string, value: string | undefined): string {
	if (!value) throw new Error(`missing broadcast connection config: ${name}`)
	return value
}

const hostname = required(
	"hostname",
	runtime.hostname ?? import.meta.env.ROOMKA_HOSTNAME,
)
const webTransportPort = required(
	"webTransportPort",
	runtime.webTransportPort ?? import.meta.env.ROOMKA_WEB_TRANSPORT_PORT,
)

// Baked-in route contract, matching the broadcast server, Caddy, and the Vite
// dev proxy.
const WEB_TRANSPORT_BASE_PATH = "/"
const API_BASE_PATH = "/api"

export const WEBTRANSPORT_URL = `https://${hostname}:${webTransportPort}${WEB_TRANSPORT_BASE_PATH}`

// The HTTP API is reached same-origin (Vite proxy in dev, Caddy in production),
// so this is a relative path. The server answers with a cert hash to pin (an
// ephemeral cert) or a null hash (a real cert — connect with normal TLS); see
// transport.ts.
export const CERT_HASH_URL = `${API_BASE_PATH}/cert-hash`

// Video codec, resolution, framerate, and bitrate are per-share choices now —
// see lib/video-presets.ts (the sharer picks a preset in Settings; the receiver
// learns the codec from the stream). Only the keyframe cadence is fixed here.
export const KEYFRAME_INTERVAL_MS = 2000

// Conservative upper bound on datagram payload size. The broadcast server
// bridges two independent QUIC connections (sharer↔broadcast and
// broadcast↔viewer), each with its
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
