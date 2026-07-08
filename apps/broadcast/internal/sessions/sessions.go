// Package sessions relays WebTransport datagrams between connected sessions:
// whatever one session sends, every other session receives, unmodified. There
// is only ever one relay, so the session set lives as package-level state
// rather than a type callers construct.
package sessions

import (
	"log"
	"sync"

	"github.com/quic-go/webtransport-go"
)

// store is the relay's set of connected sessions, guarded by mu.
var (
	mu    sync.RWMutex
	store = make(map[*webtransport.Session]struct{})
)

func add(session *webtransport.Session) int {
	mu.Lock()
	defer mu.Unlock()
	store[session] = struct{}{}
	return len(store)
}

func remove(session *webtransport.Session) int {
	mu.Lock()
	defer mu.Unlock()
	delete(store, session)
	return len(store)
}

func broadcast(from *webtransport.Session, data []byte) {
	mu.RLock()
	defer mu.RUnlock()
	for session := range store {
		if session == from {
			continue
		}
		if err := session.SendDatagram(data); err != nil {
			log.Printf("send to peer failed: %v", err)
		}
	}
}

// Handle registers session with the relay and blocks, forwarding its
// datagrams to every other session, until it disconnects.
func Handle(session *webtransport.Session) {
	log.Printf("session connected, total=%d", add(session))
	defer func() {
		log.Printf("session disconnected, total=%d", remove(session))
	}()

	ctx := session.Context()
	for {
		data, err := session.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		broadcast(session, data)
	}
}
