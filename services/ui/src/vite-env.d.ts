/// <reference types="vite/client" />

interface ImportMetaEnv {
	// Only these non-secret connection values reach the client bundle (whitelisted
	// in vite.config.ts). ROOMKA_ACCESS_SECRET must never appear here.
	readonly ROOMKA_HOSTNAME?: string
	readonly ROOMKA_WEB_TRANSPORT_PORT?: string
}

// Runtime config injected by the container via /config.js (see the entrypoint),
// so one image serves any host.
interface Window {
	__ROOMKA_CONFIG__?: {
		hostname?: string
		webTransportPort?: string
	}
}
