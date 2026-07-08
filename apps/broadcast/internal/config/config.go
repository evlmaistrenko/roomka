// Package config reads the broadcast server's settings from the environment.
// Every value comes from a ROOMKA_BROADCAST_* variable (with a sensible
// default). How those variables get set is not this program's concern: in
// production the container runtime injects them; in development the dev script
// loads them from the monorepo's root .env.
package config

import "os"

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
		Port:            env("ROOMKA_BROADCAST_PORT", "4433"),
		Route:           env("ROOMKA_BROADCAST_ROUTE", "/"),
		CertPath:        env("ROOMKA_BROADCAST_CERT_PATH", ""),
		CertKeyPath:     env("ROOMKA_BROADCAST_CERT_KEY_PATH", ""),
		DevCertHashPort: env("ROOMKA_BROADCAST_DEV_CERT_HASH_PORT", "8080"),
		// A dev placeholder (like the self-signed cert): override in production.
		JWTSecret: env("ROOMKA_BROADCAST_JWT_SECRET", "roomka-dev-jwt-secret-change-in-production"),
	}
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
