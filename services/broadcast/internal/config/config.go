// Package config reads the broadcast server's settings from the environment.
// Every value is a required ROOMKA_* variable with no default — a missing one is
// a hard startup error, so the running configuration is always explicit.
package config

import (
	"log"
	"os"
	"strings"
)

// Config holds every runtime parameter of the broadcast server. HOSTNAME is not
// here: the server binds all interfaces and its ephemeral cert is pinned by
// hash, so the hostname has no server-side effect (it's a UI/Caddy concern).
type Config struct {
	WebTransportCert string // ROOMKA_WEB_TRANSPORT_CERT: "ephemeral" or "static:<cert>;<key>"
	WebTransportPort string // ROOMKA_WEB_TRANSPORT_PORT: UDP/QUIC bind port
	APIPort          string // ROOMKA_API_PORT: HTTP API bind port
	AccessSecret     string // ROOMKA_ACCESS_SECRET: HMAC secret for access tokens
}

// Load reads the configuration from the environment, exiting if any required
// variable is unset or empty. All missing names are reported at once.
func Load() Config {
	var missing []string
	get := func(key string) string {
		value := os.Getenv(key)
		if value == "" {
			missing = append(missing, key)
		}
		return value
	}
	cfg := Config{
		WebTransportCert: get("ROOMKA_WEB_TRANSPORT_CERT"),
		WebTransportPort: get("ROOMKA_WEB_TRANSPORT_PORT"),
		APIPort:          get("ROOMKA_API_PORT"),
		AccessSecret:     get("ROOMKA_ACCESS_SECRET"),
	}
	if len(missing) > 0 {
		log.Fatalf("required environment variables not set: %s", strings.Join(missing, ", "))
	}
	return cfg
}
