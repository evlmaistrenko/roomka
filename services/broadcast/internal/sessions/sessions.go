// Package sessions relays WebTransport traffic between connected sessions:
// whatever one session sends, every other session receives, unmodified. Both
// transports QUIC offers are forwarded — unreliable datagrams (screen-share
// deltas and audio) and reliable unidirectional streams (video keyframes, one
// per stream) — but the relay never interprets a payload; it only fans bytes
// out. Each session has its own bounded outbound queues drained by dedicated
// goroutines, so a slow or stalled client can only drop its own traffic — it
// can't block delivery to the others or freeze the relay. There is only ever one
// relay, so the session set lives as package-level state rather than a type
// callers build.
package sessions

import (
	"context"
	"io"
	"log"
	"sync"

	"github.com/quic-go/webtransport-go"
)

// outboundQueueSize bounds each peer's pending datagrams. Screen-share datagrams
// are best-effort, so a peer that can't keep up drops its overflow instead of
// backpressuring the sender or the relay.
const outboundQueueSize = 64

// outboundStreamQueueSize bounds each peer's pending keyframes awaiting a stream.
// Keyframes are seconds apart, so this rarely fills; when it does (a stalled
// peer) the overflow is dropped and the peer re-syncs at the next keyframe.
const outboundStreamQueueSize = 8

// maxKeyframeStreamBytes caps a single forwarded keyframe. A real keyframe is
// well under a megabyte even at 4K; the cap bounds memory against a misbehaving
// or malicious client sending an unbounded stream. Oversized streams are dropped.
const maxKeyframeStreamBytes = 4 << 20 // 4 MiB

// maxConcurrentKeyframeReads bounds how many of a session's incoming keyframe
// streams this code reads at once (each read buffers up to maxKeyframeStreamBytes,
// so the actively-read memory is bounded by their product). Keyframes are seconds
// apart, so this headroom is only approached by a misbehaving client — which it
// then throttles, without affecting other sessions. (Streams the peer has opened
// but that we haven't accepted yet are separately bounded by the QUIC connection's
// stream limit and flow-control window.)
const maxConcurrentKeyframeReads = 4

// keyframeStreamDropCode is the application error code used to reset a keyframe
// stream we won't deliver — an oversized/errored inbound stream we drop, or an
// outbound forward that failed mid-write — so the peer stops and QUIC reclaims the
// stream's resources promptly.
const keyframeStreamDropCode webtransport.StreamErrorCode = 1

type peer struct {
	session    *webtransport.Session
	out        chan []byte // pending datagrams
	outStreams chan []byte // pending keyframes, each forwarded on its own stream
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

// snapshotTargets returns every connected peer except `from`, taken under the
// read lock and returned before any (potentially blocking) send, so no single
// peer's delivery can stall the relay or another broadcaster while enqueuing.
func snapshotTargets(from *webtransport.Session) []*peer {
	mu.RLock()
	defer mu.RUnlock()
	targets := make([]*peer, 0, len(store))
	for session, p := range store {
		if session != from {
			targets = append(targets, p)
		}
	}
	return targets
}

// broadcast fans a datagram out to every peer except `from`. The enqueue is
// non-blocking — so no single peer's send can stall the relay or another
// broadcaster (a blocking SendDatagram would, held under the lock).
func broadcast(from *webtransport.Session, data []byte) {
	for _, p := range snapshotTargets(from) {
		select {
		case p.out <- data:
		default:
			// peer is backed up — drop (delivery is best-effort).
		}
	}
}

// broadcastStream fans a keyframe out to every peer except `from`, each on its
// own reliable stream. Like broadcast, the enqueue is non-blocking: a peer whose
// keyframe queue is full drops this one (best-effort) rather than stalling the
// relay.
func broadcastStream(from *webtransport.Session, data []byte) {
	for _, p := range snapshotTargets(from) {
		select {
		case p.outStreams <- data:
		default:
			// peer's keyframe queue is full — drop (it re-syncs at the next keyframe).
		}
	}
}

// Handle registers session with the relay and blocks, forwarding its datagrams to
// every other session, until it disconnects. Incoming streams and this session's
// own outbound queues are drained by sibling goroutines for the session's life.
func Handle(session *webtransport.Session) {
	p := &peer{
		session:    session,
		out:        make(chan []byte, outboundQueueSize),
		outStreams: make(chan []byte, outboundStreamQueueSize),
	}
	log.Printf("session connected, total=%d", add(p))
	defer func() {
		log.Printf("session disconnected, total=%d", remove(session))
	}()

	ctx := session.Context()
	go writeLoop(ctx, p)
	go streamWriteLoop(ctx, p)
	go readStreamLoop(ctx, session)

	for {
		// ReceiveDatagram returns a fresh slice each call, so forwarding a received
		// datagram to several peers' queues never aliases the next one.
		data, err := session.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		broadcast(session, data)
	}
}

// writeLoop drains a peer's outbound datagram queue, sending each on its own
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
				// Best-effort: drop this datagram (e.g. it exceeds this peer's path
				// MTU, or the connection is dying). Log the transition once per failure
				// streak so it's diagnosable without flooding.
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

// streamWriteLoop drains a peer's pending keyframes, forwarding each on a fresh
// unidirectional stream. Opening a stream can block on flow control, but that
// only stalls this goroutine — the bounded queue absorbs a little and
// broadcastStream drops the rest. It exits when the session disconnects.
func streamWriteLoop(ctx context.Context, p *peer) {
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-p.outStreams:
			if err := forwardKeyframe(ctx, p.session, data); err != nil {
				if !failing {
					failing = true
					log.Printf("dropping keyframe streams to a peer: %v", err)
				}
				continue
			}
			failing = false
		}
	}
}

// forwardKeyframe opens a unidirectional stream, writes one keyframe, and closes
// it (the FIN delimits the keyframe for the receiver). A write error resets the
// stream (CancelWrite) rather than Close, so the receiver sees a reset and drops
// the partial instead of mistaking a clean FIN for a complete keyframe.
func forwardKeyframe(ctx context.Context, session *webtransport.Session, data []byte) error {
	stream, err := session.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}
	if _, err := stream.Write(data); err != nil {
		stream.CancelWrite(keyframeStreamDropCode)
		return err
	}
	return stream.Close()
}

// readStreamLoop accepts this session's incoming unidirectional streams and fans
// each out to the other peers. Every stream carries exactly one keyframe, read to
// completion (its FIN) before forwarding; reads run in their own goroutine so a
// slow one doesn't hold up the next accept. It exits when the session disconnects.
func readStreamLoop(ctx context.Context, session *webtransport.Session) {
	// A bounded set of read slots caps concurrent reads (and thus buffered memory);
	// the accept loop only blocks here once a session has more keyframe streams
	// in flight than any well-behaved client produces.
	slots := make(chan struct{}, maxConcurrentKeyframeReads)
	for {
		stream, err := session.AcceptUniStream(ctx)
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		go func(stream *webtransport.ReceiveStream) {
			defer func() { <-slots }()
			data, ok := readKeyframe(stream)
			if !ok {
				// Reset the stream so the peer stops sending and QUIC reclaims its
				// stream credit and buffered bytes, instead of leaving it dangling
				// (unread, un-FIN'd) until the session ends.
				stream.CancelRead(keyframeStreamDropCode)
				return
			}
			broadcastStream(session, data)
		}(stream)
	}
}

// readKeyframe reads a whole keyframe stream into a fresh slice, or reports false
// if the stream errors, is empty, or exceeds the size cap (reading one byte past
// the cap distinguishes an oversized stream from one that's exactly at it).
func readKeyframe(stream *webtransport.ReceiveStream) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(stream, maxKeyframeStreamBytes+1))
	if err != nil {
		return nil, false
	}
	if len(data) == 0 || len(data) > maxKeyframeStreamBytes {
		return nil, false
	}
	return data, true
}
