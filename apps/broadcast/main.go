// Command broadcast is a dumb WebTransport relay: it accepts datagrams from
// any connected client and forwards each one, unmodified, to every other
// connected client. It does not interpret, buffer, or persist payloads.
package main

import (
	"crypto/tls"
	"log"
	"net/http"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"broadcast/internal/auth"
	"broadcast/internal/certs"
	"broadcast/internal/config"
	"broadcast/internal/dev"
	"broadcast/internal/sessions"
)

func main() {
	cfg := config.Load()
	listenAddress := ":" + cfg.Port

	var tlsConfig *tls.Config
	switch {
	case cfg.CertPath == "" && cfg.CertKeyPath == "":
		var certHash string
		tlsConfig, certHash = dev.GenerateConfig()
		log.Printf("dev mode: using an ephemeral self-signed certificate")
		log.Printf("certificate hash (sha-256, base64), for serverCertificateHashes: %s", certHash)
		go dev.ServeHash(":"+cfg.DevCertHashPort, certHash)
	case cfg.CertPath != "" && cfg.CertKeyPath != "":
		var err error
		tlsConfig, err = certs.LoadConfig(cfg.CertPath, cfg.CertKeyPath)
		if err != nil {
			log.Fatalf("failed to load TLS certificate: %v", err)
		}
		log.Printf("production mode: serving TLS certificate from %s (auto-reloaded on change)", cfg.CertPath)
	default:
		log.Fatal("ROOMKA_BROADCAST_CERT_PATH and ROOMKA_BROADCAST_CERT_KEY_PATH must both be set")
	}

	http3Server := &http3.Server{
		Addr:      listenAddress,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
	}
	webtransport.ConfigureHTTP3Server(http3Server)

	webTransportServer := &webtransport.Server{
		H3: http3Server,
		// Dumb dev relay: accept connections from any origin.
		CheckOrigin: func(*http.Request) bool { return true },
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Route, func(w http.ResponseWriter, r *http.Request) {
		// Browsers can't set headers on the WebTransport handshake, so the JWT
		// rides in the query string. Verify it before upgrading — an invalid or
		// expired token gets a 401 and the session is never established.
		if err := auth.Verify(r.URL.Query().Get("token"), cfg.JWTSecret); err != nil {
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

	log.Printf("webtransport broadcast server listening on udp%s (route %s)", listenAddress, cfg.Route)
	if err := webTransportServer.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
