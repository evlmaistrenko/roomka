// Package dev provides the development-mode TLS setup: an ephemeral,
// self-signed cert regenerated on each start, plus a small HTTP endpoint that
// serves its hash for browser serverCertificateHashes pinning.
package dev

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"
)

// GenerateConfig creates an ephemeral, self-signed cert for local development
// and returns it together with its SHA-256 hash (base64). Clients that can't
// rely on a trusted CA (e.g. browsers talking to localhost) pin it via the
// serverCertificateHashes API, which requires the cert to be valid for no more
// than two weeks.
func GenerateConfig() (*tls.Config, string) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("failed to generate certificate key: %v", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(13 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		log.Fatalf("failed to create certificate: %v", err)
	}

	hash := sha256.Sum256(certDER)
	certHash := base64.StdEncoding.EncodeToString(hash[:])

	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{certDER},
			PrivateKey:  privateKey,
		}},
	}, certHash
}

// ServeHash exposes the dev cert's hash over plain HTTP so a browser can fetch
// it out of band (before the WebTransport handshake, since the self-signed
// cert isn't otherwise trusted) and pass it to serverCertificateHashes. Only
// used in dev; production certs are CA-trusted and need no pinning.
func ServeHash(address, certHash string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cert-hash", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"algorithm": "sha-256",
			"hash":      certHash,
		})
	})
	log.Printf("dev cert-hash endpoint listening on http://localhost%s/cert-hash", address)
	if err := http.ListenAndServe(address, mux); err != nil {
		log.Printf("dev cert-hash endpoint failed: %v", err)
	}
}
