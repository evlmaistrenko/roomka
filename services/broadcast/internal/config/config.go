// Package config reads the broadcast server's settings from the environment.
// Non-secret connection values shared with the UI come from ROOMKA_PUBLIC_*
// variables (the UI reads the same ones — see services/ui), while server-only
// secrets and file paths come from ROOMKA_BROADCAST_* (never exposed to the
// client). Each has a sensible default. How those variables get set is not this
// program's concern: in production the container runtime injects them; in
// development the dev script loads them from the monorepo's root .env.
package config

import "os"

// DevJWTSecret is the well-known placeholder secret used only in development
// (when ROOMKA_BROADCAST_JWT_SECRET is unset and no production certs are
// configured). It is intentionally public — production must never use it, and
// main refuses to start if it is in effect while serving real certificates.
const DevJWTSecret = "roomka-dev-jwt-secret-change-in-production"

// Config holds every runtime parameter of the broadcast server.
type Config struct {
	Port            string // UDP/QUIC port for WebTransport; bound on all interfaces
	Route           string // HTTP route the WebTransport CONNECT upgrade is mounted on
	CertPath        string // production PEM certificate file; empty enables dev mode
	CertKeyPath     string // production PEM private key file, paired with CertPath
	DevCertHashPort string // dev mode: plain-HTTP port serving the cert hash
	JWTSecret       string // HMAC secret verifying the JWT that gates connections
}

// Load reads the configuration from the environment.
func Load() Config {
	return Config{
		Port:            env("ROOMKA_PUBLIC_BROADCAST_PORT", "4433"),
		Route:           env("ROOMKA_PUBLIC_BROADCAST_ROUTE", "/"),
		CertPath:        env("ROOMKA_BROADCAST_CERT_PATH", ""),
		CertKeyPath:     env("ROOMKA_BROADCAST_CERT_KEY_PATH", ""),
		DevCertHashPort: env("ROOMKA_PUBLIC_BROADCAST_CERT_HASH_PORT", "8080"),
		// No default: production must set a strong secret, and dev falls back to
		// DevJWTSecret explicitly (see main). An empty value here is the signal
		// that nothing was configured.
		JWTSecret: env("ROOMKA_BROADCAST_JWT_SECRET", ""),
	}
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
