// Placeholder runtime config. In dev this stays empty and the app falls back to
// build-time env / defaults (see src/lib/config.ts). In the production container
// the entrypoint overwrites this file with values from env at startup.
// @ts-ignore
window.__ROOMKA_CONFIG__ = window.__ROOMKA_CONFIG__ || {}
