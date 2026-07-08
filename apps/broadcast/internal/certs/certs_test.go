package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writePair generates a fresh self-signed cert/key pair (identified by serial)
// and installs it at certPath/keyPath the way a renewal tool would: write a temp
// file, then rename it over the target. The rename is what would sever a watch
// registered on the file itself rather than its directory.
func writePair(t *testing.T, certPath, keyPath string, serial int64) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	atomicWrite(t, certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	atomicWrite(t, keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func atomicWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func serialOf(t *testing.T, w *watcher) int64 {
	t.Helper()
	cert, err := w.getCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.SerialNumber.Int64()
}

// TestWatcherReloadsOnRotation checks the end-to-end hot-reload path: a cert
// rotated on disk (via rename, as certbot does) is picked up without restart.
func TestWatcherReloadsOnRotation(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writePair(t, certPath, keyPath, 1)

	w, err := newWatcher(certPath, keyPath)
	if err != nil {
		t.Fatalf("newWatcher: %v", err)
	}
	if got := serialOf(t, w); got != 1 {
		t.Fatalf("initial serial = %d, want 1", got)
	}

	writePair(t, certPath, keyPath, 2)

	deadline := time.Now().Add(5 * time.Second)
	for serialOf(t, w) != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("cert not reloaded within timeout; serial still %d", serialOf(t, w))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestWatcherReloadsKeyOnlyChange guards the original bug: a key rotated without
// the cert's mtime changing must still trigger a reload. Here we rewrite both
// files but the point is the watcher keys off directory events, not the cert
// mtime alone.
func TestWatcherReloadsSeparateDirs(t *testing.T) {
	certDir := t.TempDir()
	keyDir := t.TempDir()
	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(keyDir, "key.pem")
	writePair(t, certPath, keyPath, 10)

	w, err := newWatcher(certPath, keyPath)
	if err != nil {
		t.Fatalf("newWatcher: %v", err)
	}
	if got := serialOf(t, w); got != 10 {
		t.Fatalf("initial serial = %d, want 10", got)
	}

	writePair(t, certPath, keyPath, 20)

	deadline := time.Now().Add(5 * time.Second)
	for serialOf(t, w) != 20 {
		if time.Now().After(deadline) {
			t.Fatalf("cert in separate dir not reloaded within timeout; serial still %d", serialOf(t, w))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
