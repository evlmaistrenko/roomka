// End-to-end encryption of the media payload (the encoded video/audio bytes)
// with AES-GCM. The relay only ever sees ciphertext. The datagram header
// (senderId/frameId/timestamp/flags) stays in the clear — it's routing
// metadata, not content.
//
// The key comes from the imported settings file (see settings.ts), not a
// hardcoded constant. Every peer in a room must import the same key to interop.
import { importAesKey, loadSettings } from './settings'

const IV_LENGTH = 12 // AES-GCM standard nonce length

let keyPromise: Promise<CryptoKey> | null = null

export function getKey(): Promise<CryptoKey> {
  keyPromise ??= (async () => {
    const settings = loadSettings()
    if (!settings) {
      throw new Error('No E2EE key configured — import a settings file first.')
    }
    return importAesKey(settings.e2eeKey)
  })()
  return keyPromise
}

// Clear the cached key so a newly imported settings file takes effect.
export function resetKey() {
  keyPromise = null
}

// encrypt returns [12-byte random IV][ciphertext+tag]. A fresh IV per call is
// required for AES-GCM.
export async function encrypt(
  key: CryptoKey,
  plaintext: Uint8Array,
): Promise<Uint8Array> {
  const iv = crypto.getRandomValues(new Uint8Array(IV_LENGTH))
  // Our buffers are always ArrayBuffer-backed; cast to satisfy the stricter
  // BufferSource typing.
  const ciphertext = new Uint8Array(
    await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv },
      key,
      plaintext as BufferSource,
    ),
  )
  const out = new Uint8Array(IV_LENGTH + ciphertext.byteLength)
  out.set(iv, 0)
  out.set(ciphertext, IV_LENGTH)
  return out
}

// decrypt reverses encrypt; returns null if authentication fails (corrupt or
// wrong key), so the caller can drop the frame.
export async function decrypt(
  key: CryptoKey,
  payload: Uint8Array,
): Promise<Uint8Array | null> {
  const iv = payload.subarray(0, IV_LENGTH)
  const ciphertext = payload.subarray(IV_LENGTH)
  try {
    return new Uint8Array(
      await crypto.subtle.decrypt(
        { name: 'AES-GCM', iv: iv as BufferSource },
        key,
        ciphertext as BufferSource,
      ),
    )
  } catch {
    return null
  }
}
