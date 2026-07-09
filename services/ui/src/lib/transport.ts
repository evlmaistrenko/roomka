import { CERT_HASH_URL, WEBTRANSPORT_URL } from "./config"
import { loadSettings } from "./settings"

function base64ToBytes(base64: string): Uint8Array<ArrayBuffer> {
	const binary = atob(base64)
	const bytes = new Uint8Array(new ArrayBuffer(binary.length))
	for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
	return bytes
}

// connect opens a WebTransport session to the broadcast relay. In dev the
// server uses an ephemeral self-signed certificate, so we fetch its SHA-256
// hash out of band and pin it via serverCertificateHashes. The relay authorizes
// the connection by a JWT — browsers can't set handshake headers, so it rides in
// the URL query string (the relay reads ?token=... and verifies it).
export async function connect(): Promise<WebTransport> {
	const settings = loadSettings()
	if (!settings) {
		throw new Error("No connection token — import a settings file first.")
	}

	const response = await fetch(CERT_HASH_URL)
	if (!response.ok) {
		throw new Error(`cert-hash fetch failed: ${response.status}`)
	}
	const { hash } = (await response.json()) as {
		algorithm: string
		hash: string
	}

	const url = new URL(WEBTRANSPORT_URL)
	url.searchParams.set("token", settings.token)

	const transport = new WebTransport(url.toString(), {
		serverCertificateHashes: [
			{ algorithm: "sha-256", value: base64ToBytes(hash) },
		],
	})
	await transport.ready
	return transport
}
