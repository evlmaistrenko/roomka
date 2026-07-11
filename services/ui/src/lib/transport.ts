import { CERT_HASH_URL, WEBTRANSPORT_URL } from "./config"
import { loadSettings } from "./settings"

function base64ToBytes(base64: string): Uint8Array<ArrayBuffer> {
	const binary = atob(base64)
	const bytes = new Uint8Array(new ArrayBuffer(binary.length))
	for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
	return bytes
}

// connect opens a WebTransport session to the broadcast server. The server
// authorizes the connection by a token — browsers can't set handshake headers,
// so it rides in the URL query string (?token=...). We always ask the server
// whether to pin its certificate: /api/cert-hash returns a SHA-256 hash for an
// ephemeral self-signed cert (we pin it via serverCertificateHashes) or a null
// hash for a real CA cert (we connect with normal TLS validation).
export async function connect(): Promise<WebTransport> {
	const settings = loadSettings()
	if (!settings) {
		throw new Error("No connection token — import a settings file first.")
	}

	const url = new URL(WEBTRANSPORT_URL)
	url.searchParams.set("token", settings.token)

	const options: WebTransportOptions = {}
	// Ask the server whether to pin: a hash means an ephemeral self-signed cert
	// (pin it); a null hash means a real CA cert (normal TLS validation).
	const response = await fetch(CERT_HASH_URL)
	if (!response.ok) {
		throw new Error(`cert-hash fetch failed: ${response.status}`)
	}
	const { hash } = (await response.json()) as {
		algorithm?: string
		hash: string | null
	}
	if (hash) {
		options.serverCertificateHashes = [
			{ algorithm: "sha-256", value: base64ToBytes(hash) },
		]
	}

	const transport = new WebTransport(url.toString(), options)
	await transport.ready
	return transport
}
