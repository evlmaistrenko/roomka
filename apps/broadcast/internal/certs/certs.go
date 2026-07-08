// Package certs loads the server's TLS certificate from disk for production,
// hot-reloading it when the files change. Renewal (e.g. via certbot) happens
// out-of-band, and a relay shouldn't need a restart - which would drop every
// connected client - to pick up the new cert.
package certs

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// LoadConfig returns a TLS config that serves the cert/key pair from disk,
// reloading it in the background whenever the files change.
func LoadConfig(certFile, keyFile string) (*tls.Config, error) {
	w, err := newWatcher(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{GetCertificate: w.getCertificate}, nil
}

// watcher keeps a cert/key pair loaded in memory, reloading it when the files
// on disk change so renewal doesn't require restarting the process.
type watcher struct {
	certFile, keyFile string

	mu   sync.RWMutex
	cert *tls.Certificate
}

func newWatcher(certFile, keyFile string) (*watcher, error) {
	w := &watcher{certFile: certFile, keyFile: keyFile}
	if err := w.reload(); err != nil {
		return nil, err
	}
	go w.watch()
	return w, nil
}

func (w *watcher) reload() error {
	cert, err := tls.LoadX509KeyPair(w.certFile, w.keyFile)
	if err != nil {
		return fmt.Errorf("loading TLS certificate: %w", err)
	}
	w.mu.Lock()
	w.cert = &cert
	w.mu.Unlock()
	return nil
}

func (w *watcher) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert, nil
}

func (w *watcher) watch() {
	const pollInterval = 30 * time.Second

	lastModTime := time.Time{}
	if info, err := os.Stat(w.certFile); err == nil {
		lastModTime = info.ModTime()
	}

	for {
		time.Sleep(pollInterval)

		info, err := os.Stat(w.certFile)
		if err != nil {
			log.Printf("cert watcher: stat %s failed: %v", w.certFile, err)
			continue
		}
		if info.ModTime().Equal(lastModTime) {
			continue
		}
		if err := w.reload(); err != nil {
			log.Printf("cert watcher: reload failed: %v", err)
			continue
		}
		lastModTime = info.ModTime()
		log.Printf("TLS certificate reloaded from %s", w.certFile)
	}
}
