// Package certs loads the server's TLS certificate from disk for production,
// hot-reloading it when the files change. Renewal (e.g. via certbot) happens
// out-of-band, and a broadcast server shouldn't need a restart - which would
// drop every connected client - to pick up the new cert.
package certs

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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
	fsw               *fsnotify.Watcher

	mu      sync.RWMutex
	cert    *tls.Certificate
	reloads int // successful (re)loads; a test seam, guarded by mu
}

func newWatcher(certFile, keyFile string) (*watcher, error) {
	w := &watcher{certFile: certFile, keyFile: keyFile}
	if err := w.reload(); err != nil {
		return nil, err
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating cert file watcher: %w", err)
	}
	w.fsw = fsw
	// Watch the parent directories rather than the files themselves: renewal
	// tools typically install a new cert by writing a temp file and renaming it
	// over the old one, which moves the inode and would sever a watch registered
	// directly on the file. A directory watch still observes the create/rename,
	// and covers a key that rotates without the cert's path changing.
	for _, dir := range uniqueDirs(w.certFile, w.keyFile) {
		if err := fsw.Add(dir); err != nil {
			_ = fsw.Close()
			return nil, fmt.Errorf("watching %s: %w", dir, err)
		}
	}
	go w.watch()
	return w, nil
}

// Close stops watching and releases the OS handle. Optional in production (the
// watcher runs for the process lifetime); tests call it to avoid leaking the
// goroutine and inotify/ReadDirectoryChangesW handle.
func (w *watcher) Close() error {
	return w.fsw.Close()
}

func (w *watcher) reload() error {
	cert, err := tls.LoadX509KeyPair(w.certFile, w.keyFile)
	if err != nil {
		return fmt.Errorf("loading TLS certificate: %w", err)
	}
	warnIfInsecureKeyPerm(w.keyFile)
	w.mu.Lock()
	w.cert = &cert
	w.reloads++
	w.mu.Unlock()
	return nil
}

// reloadCount reports how many times the pair has been (re)loaded (test seam).
func (w *watcher) reloadCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.reloads
}

func (w *watcher) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert, nil
}

// watch reloads the pair whenever fsnotify reports a change to either file. A
// renewal rewrites the cert and key as a burst of events a moment apart, so it
// debounces: each relevant event (re)arms a short timer and the reload runs
// only once the burst goes quiet, so we never load a half-written pair.
func (w *watcher) watch() {
	const (
		debounce   = 500 * time.Millisecond
		maxRetries = 5
	)
	timer := time.NewTimer(debounce)
	timer.Stop()
	retries := 0

	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if w.relevant(event.Name) {
				retries = 0
				timer.Reset(debounce)
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("cert watcher: %v", err)
		case <-timer.C:
			if err := w.reload(); err != nil {
				// Likely caught mid-renewal (one file written, its pair not yet).
				// Retry a few times, then fall silent and wait for the next event
				// so a genuinely broken cert doesn't spin the log.
				if retries++; retries <= maxRetries {
					log.Printf("cert watcher: reload failed (attempt %d/%d), retrying: %v", retries, maxRetries, err)
					timer.Reset(debounce)
				} else {
					log.Printf("cert watcher: reload still failing after %d attempts, waiting for next change: %v", maxRetries, err)
				}
				continue
			}
			retries = 0
			log.Printf("TLS certificate reloaded from %s", w.certFile)
		}
	}
}

// relevant reports whether an event path refers to the cert or key file.
func (w *watcher) relevant(name string) bool {
	clean := filepath.Clean(name)
	return clean == filepath.Clean(w.certFile) || clean == filepath.Clean(w.keyFile)
}

// uniqueDirs returns the deduplicated parent directories of the given files.
func uniqueDirs(files ...string) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// warnIfInsecureKeyPerm logs a warning when the private key file is readable by
// group or others; TLS keys should be 0600/0400. Unix-only — on Windows the
// mode bits are synthetic, so the check would only produce false positives.
func warnIfInsecureKeyPerm(keyFile string) {
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(keyFile)
	if err != nil {
		return
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		log.Printf("cert watcher: WARNING: private key %s is group/world-accessible (mode %04o); tighten to 0600", keyFile, perm)
	}
}
