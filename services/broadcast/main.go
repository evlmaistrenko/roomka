// Command broadcast is a dumb WebTransport broadcast server: it accepts
// datagrams from any connected client and forwards each one, unmodified, to
// every other connected client. It does not interpret, buffer, or persist
// payloads. Alongside it runs a small HTTP API (see internal/api) that a reverse
// proxy fronts, so the browser can reach it same-origin.
package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"strings"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"broadcast/internal/api"
	"broadcast/internal/auth"
	"broadcast/internal/certs"
	"broadcast/internal/config"
	"broadcast/internal/selfsigned"
	"broadcast/internal/sessions"
)

// webTransportBasePath is the route the WebTransport CONNECT upgrade is mounted
// on — a baked-in route contract, mirrored by the UI.
const webTransportBasePath = "/"

func main() {
	cfg := config.Load()

	tlsConfig, certHash := webTransportTLS(cfg.WebTransportCert)

	// The HTTP API is always on (cert-hash now, more later); a reverse proxy
	// fronts it — Vite in development, Caddy in the container — so the browser
	// reaches it same-origin.
	go api.Serve(":"+cfg.APIPort, certHash)

	http3Server := &http3.Server{
		Addr:      ":" + cfg.WebTransportPort,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
	}
	webtransport.ConfigureHTTP3Server(http3Server)

	webTransportServer := &webtransport.Server{
		H3: http3Server,
		// Accept connections from any origin; the access token gates them.
		CheckOrigin: func(*http.Request) bool { return true },
	}

	mux := http.NewServeMux()
	mux.HandleFunc(webTransportBasePath, func(w http.ResponseWriter, r *http.Request) {
		// Browsers can't set headers on the WebTransport handshake, so the token
		// rides in the query string. Verify it before upgrading — an invalid or
		// expired token gets a 401 and the session is never established.
		if err := auth.Verify(r.URL.Query().Get("token"), cfg.AccessSecret); err != nil {
			log.Printf("rejected connection: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		session, err := webTransportServer.Upgrade(w, r)
		if err != nil {
			log.Printf("upgrade failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sessions.Handle(session)
	})
	http3Server.Handler = mux

	log.Printf("webtransport server listening on udp:%s", cfg.WebTransportPort)
	if err := webTransportServer.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// webTransportTLS builds the WebTransport server's TLS config from the
// ROOMKA_WEB_TRANSPORT_CERT spec. "ephemeral" generates a self-signed cert and
// returns its base64 SHA-256 hash (the browser pins it); "static:<cert>;<key>"
// loads a real pair and returns an empty hash (no pinning). Anything else is a
// fatal misconfiguration.
func webTransportTLS(spec string) (*tls.Config, string) {
	if spec == "ephemeral" {
		tlsConfig, certHash := selfsigned.GenerateConfig()
		log.Printf("web_transport_cert=ephemeral: serving a self-signed certificate")
		log.Printf("certificate hash (sha-256, base64), for serverCertificateHashes: %s", certHash)
		return tlsConfig, certHash
	}

	pair, ok := strings.CutPrefix(spec, "static:")
	if !ok {
		log.Fatalf("ROOMKA_WEB_TRANSPORT_CERT must be \"ephemeral\" or \"static:<cert>;<key>\", got %q", spec)
	}
	certFile, keyFile, ok := strings.Cut(pair, ";")
	if !ok || certFile == "" || keyFile == "" {
		log.Fatalf("ROOMKA_WEB_TRANSPORT_CERT static form must be \"static:<cert>;<key>\", got %q", spec)
	}
	tlsConfig, err := certs.LoadConfig(certFile, keyFile)
	if err != nil {
		log.Fatalf("failed to load TLS certificate: %v", err)
	}
	log.Printf("web_transport_cert=static: serving %s (auto-reloaded on change)", certFile)
	return tlsConfig, ""
}
