// Package api serves the broadcast server's HTTP API. It is always on; a reverse
// proxy fronts it (Vite in development, Caddy in the container) so the browser
// reaches it same-origin under BasePath.
package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// BasePath is the route prefix every API endpoint sits under — a baked-in route
// contract, mirrored by the UI and the reverse proxies.
const BasePath = "/api"

// Serve starts the HTTP API on addr and blocks. certHash is the base64 SHA-256
// of the WebTransport server's self-signed certificate when it uses an ephemeral
// cert (the browser pins it); it is empty when a real cert is served, and
// /cert-hash then reports a null hash so the client skips pinning.
func Serve(addr, certHash string) {
	mux := http.NewServeMux()
	mux.HandleFunc(BasePath+"/cert-hash", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if certHash == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"hash": nil})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"algorithm": "sha-256",
			"hash":      certHash,
		})
	})
	log.Printf("http api listening on %s (base path %s)", addr, BasePath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http api failed: %v", err)
	}
}
