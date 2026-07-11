// Package selfsigned generates an ephemeral, self-signed TLS certificate for the
// WebTransport server, returned with the SHA-256 hash the browser pins via the
// serverCertificateHashes API (which requires the cert to be valid for at most
// two weeks). It replaces a real CA cert when ROOMKA_WEB_TRANSPORT_CERT is
// "ephemeral".
package selfsigned

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"log"
	"math/big"
	"net"
	"time"
)

// GenerateConfig creates an ephemeral, self-signed cert (valid for localhost)
// and returns it together with its SHA-256 hash (base64). Clients that can't
// rely on a trusted CA pin it via serverCertificateHashes.
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
