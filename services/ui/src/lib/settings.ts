// App settings imported from a JSON file at startup and cached in localStorage:
//   - e2eeKey: base64 of a raw 128/192/256-bit AES key, used to encrypt/decrypt
//     the media payload (see e2ee.ts).
//   - token:   a JWT presented to the broadcast server to authorize the
//     WebTransport connection (see transport.ts); the client only
//     sanity-checks its shape — the broadcast server verifies the signature.

const STORAGE_KEY = "roomka:settings"

export type Settings = {
	e2eeKey: string
	token: string
}

function isJwtShaped(value: unknown): value is string {
	if (typeof value !== "string") return false
	const parts = value.split(".")
	return parts.length === 3 && parts.every((part) => part.length > 0)
}

export function loadSettings(): Settings | null {
	const raw = localStorage.getItem(STORAGE_KEY)
	if (!raw) return null
	try {
		const parsed = JSON.parse(raw) as Partial<Settings>
		if (
			typeof parsed.e2eeKey === "string" &&
			parsed.e2eeKey.length > 0 &&
			isJwtShaped(parsed.token)
		) {
			return { e2eeKey: parsed.e2eeKey, token: parsed.token }
		}
	} catch {
		// corrupt entry — treat as no settings
	}
	return null
}

export function saveSettings(settings: Settings) {
	localStorage.setItem(STORAGE_KEY, JSON.stringify(settings))
}

export function clearSettings() {
	localStorage.removeItem(STORAGE_KEY)
}

// Decode a base64 raw AES key and import it as an AES-GCM CryptoKey. Throws a
// human-readable Error on malformed base64 or an unsupported length, so callers
// can surface exactly why a key is invalid.
export async function importAesKey(base64Key: string): Promise<CryptoKey> {
	let bytes: Uint8Array
	try {
		const binary = atob(base64Key.trim())
		bytes = new Uint8Array(binary.length)
		for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
	} catch {
		throw new Error("E2EE key is not valid base64.")
	}
	if (
		bytes.byteLength !== 16 &&
		bytes.byteLength !== 24 &&
		bytes.byteLength !== 32
	) {
		throw new Error(
			`E2EE key must be a 128, 192, or 256-bit key (got ${bytes.byteLength * 8} bits).`,
		)
	}
	return crypto.subtle.importKey(
		"raw",
		bytes as BufferSource,
		"AES-GCM",
		false,
		["encrypt", "decrypt"],
	)
}

// Parse and validate a settings file's text. Throws a human-readable Error when
// the JSON is malformed, the shape is wrong, or the e2ee key isn't a usable AES
// key — so the import page can show the message to the user.
export async function parseSettingsFile(text: string): Promise<Settings> {
	let json: unknown
	try {
		json = JSON.parse(text)
	} catch {
		throw new Error("Not a valid JSON file.")
	}
	if (typeof json !== "object" || json === null) {
		throw new Error("Settings file must be a JSON object.")
	}
	const e2eeKey = (json as Record<string, unknown>).e2eeKey
	if (typeof e2eeKey !== "string" || e2eeKey.length === 0) {
		throw new Error('Settings file is missing the "e2eeKey" field.')
	}
	await importAesKey(e2eeKey) // throws if the key is invalid

	const token = (json as Record<string, unknown>).token
	if (!isJwtShaped(token)) {
		throw new Error('Settings file is missing a valid "token".')
	}
	return { e2eeKey, token }
}
