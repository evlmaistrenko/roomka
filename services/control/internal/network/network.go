// Package network answers where a request came from, which every per-address
// limit and every recorded session origin depends on.
package network

import (
	"net"
	"net/http"
	"strings"
)

// ClientAddress is the address of the client behind a request.
//
// X-Forwarded-For is a header any client can send, so it is believed only when
// the connection itself comes from loopback: the reverse proxy runs in the same
// container (Caddy in production, Vite in development), and nothing else reaches
// this server from there. Even then only the last entry is used, the one the
// proxy appended itself; anything before it is whatever the client claimed.
func ClientAddress(request *http.Request) string {
	peer := request.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if !isLoopback(peer) {
		return peer
	}
	forwarded := request.Header.Values("X-Forwarded-For")
	if len(forwarded) == 0 {
		return peer
	}
	entries := strings.Split(forwarded[len(forwarded)-1], ",")
	if last := strings.TrimSpace(entries[len(entries)-1]); last != "" {
		return last
	}
	return peer
}

func isLoopback(address string) bool {
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}
