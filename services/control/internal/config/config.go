// Package config reads the control server's settings from the environment. Every
// value is a required ROOMKA_* variable with no default — a missing one is a hard
// startup error, so the running configuration is always explicit. The few
// optional ones are marked where they are read.
package config

import (
	"cmp"
	"log"
	"os"
	"strconv"
	"strings"

	"control/internal/ratelimit"
)

// Config holds every runtime parameter of the control server.
type Config struct {
	APIPort      string // ROOMKA_API_PORT: HTTP/GraphQL bind port
	DatabasePath string // ROOMKA_DATABASE_PATH: sqlite file path
	PublicURL    string // ROOMKA_PUBLIC_URL: base URL the UI is served from; the only Origin allowed to send the session cookie
	Version      string // ROOMKA_VERSION: reported by Query.version; "dev" when unset
	// ROOMKA_DISABLE_GRAPHQL_INTROSPECTION: turns introspection off on every
	// transport, which a proxy cannot do (a query over the subscription socket
	// never shows it one). Phrased as a disable so that the zero value, and so
	// a Config built in code, keeps the default.
	DisableGraphQLIntrospection bool
	// Limits are the rate limits, which have no variables: they are tuned in
	// code, where the numbers sit side by side, not per deployment.
	Limits ratelimit.Policy
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
	configuration := Config{
		APIPort:      get("ROOMKA_API_PORT"),
		DatabasePath: get("ROOMKA_DATABASE_PATH"),
		PublicURL:    get("ROOMKA_PUBLIC_URL"),
		// Version is optional, so it bypasses the required-variable check: the
		// release build stamps it, and anything else is a development build.
		Version: cmp.Or(os.Getenv("ROOMKA_VERSION"), "dev"),
		// Optional too: unset means introspection stays on.
		DisableGraphQLIntrospection: optionalBool("ROOMKA_DISABLE_GRAPHQL_INTROSPECTION"),
		Limits:                      ratelimit.DefaultPolicy(),
	}
	if len(missing) > 0 {
		log.Fatalf("required environment variables not set: %s", strings.Join(missing, ", "))
	}
	return configuration
}

// optionalBool reads a flag that is false when unset. A value that is set but is
// not a boolean stops the server rather than reading as false: a typo in a flag
// that closes something must not leave it open without a word.
func optionalBool(key string) bool {
	value := os.Getenv(key)
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("%s must be true or false, not %q", key, value)
	}
	return parsed
}
