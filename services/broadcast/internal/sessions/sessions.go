// Package sessions relays WebTransport datagrams between connected sessions:
// whatever one session sends, every other session receives, unmodified. Each
// session has its own bounded outbound queue drained by a dedicated goroutine,
// so a slow or stalled client can only drop its own datagrams — it can't block
// delivery to the others or freeze the relay. There is only ever one relay, so
// the session set lives as package-level state rather than a type callers build.
package sessions

import (
	"context"
	"log"
	"sync"

	"github.com/quic-go/webtransport-go"
)

// outboundQueueSize bounds each peer's pending datagrams. Screen-share datagrams
// are best-effort, so a peer that can't keep up drops its overflow instead of
// backpressuring the sender or the relay.
const outboundQueueSize = 64

type peer struct {
	session *webtransport.Session
	out     chan []byte
}

// store is the relay's set of connected peers, guarded by mu.
var (
	mu    sync.RWMutex
	store = make(map[*webtransport.Session]*peer)
)

func add(p *peer) int {
	mu.Lock()
	defer mu.Unlock()
	store[p.session] = p
	return len(store)
}

func remove(session *webtransport.Session) int {
	mu.Lock()
	defer mu.Unlock()
	delete(store, session)
	return len(store)
}

// broadcast fans data out to every peer except `from`. It snapshots the targets
// under the read lock and releases it before enqueuing, and the enqueue is
// non-blocking — so no single peer's send can stall the relay or another
// broadcaster (a blocking SendDatagram would, held under the lock).
func broadcast(from *webtransport.Session, data []byte) {
	mu.RLock()
	targets := make([]*peer, 0, len(store))
	for session, p := range store {
		if session != from {
			targets = append(targets, p)
		}
	}
	mu.RUnlock()

	for _, p := range targets {
		select {
		case p.out <- data:
		default:
			// peer is backed up — drop (delivery is best-effort).
		}
	}
}

// Handle registers session with the relay and blocks, forwarding its datagrams
// to every other session, until it disconnects.
func Handle(session *webtransport.Session) {
	p := &peer{session: session, out: make(chan []byte, outboundQueueSize)}
	log.Printf("session connected, total=%d", add(p))
	defer func() {
		log.Printf("session disconnected, total=%d", remove(session))
	}()

	ctx := session.Context()
	go writeLoop(ctx, p)

	for {
		// ReceiveDatagram returns a fresh slice each call, so forwarding a
		// received datagram to several peers' queues never aliases the next one.
		data, err := session.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		broadcast(session, data)
	}
}

// writeLoop drains a peer's outbound queue, sending each datagram on its own
// connection. SendDatagram can block when that connection is congested, but that
// only stalls this goroutine — the bounded queue absorbs a little and broadcast
// drops the rest. It exits when the session's context is cancelled (disconnect).
func writeLoop(ctx context.Context, p *peer) {
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-p.out:
			if err := p.session.SendDatagram(data); err != nil {
				// Best-effort: drop this datagram (e.g. it exceeds this peer's
				// path MTU, or the connection is dying). Log the transition once
				// per failure streak so it's diagnosable without flooding.
				if !failing {
					failing = true
					log.Printf("dropping datagrams to a peer: %v", err)
				}
				continue
			}
			failing = false
		}
	}
}
