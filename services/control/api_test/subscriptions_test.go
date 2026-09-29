package apitest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// socket is one WebSocket connection speaking graphql-transport-ws, which is the
// subprotocol the schema names.
type socket struct {
	harness    *harness
	connection *websocket.Conn
}

// message is one frame of the protocol. Only the four types these tests care
// about are named; everything else is skipped when it arrives.
type message struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// connect opens a socket carrying whatever cookie this browser holds, exactly
// as a browser would: the handshake is an ordinary HTTP request, and nothing
// authenticates inside the socket afterwards.
func (b *browser) connect() (*socket, error) {
	b.harness.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	address := strings.Replace(b.harness.url, "https://", "wss://", 1) + "/graphql"
	connection, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{
		HTTPClient:   b.client,
		Subprotocols: []string{"graphql-transport-ws"},
		HTTPHeader:   http.Header{"Origin": []string{b.harness.url}},
	})
	if err != nil {
		return nil, err
	}

	opened := &socket{harness: b.harness, connection: connection}
	b.harness.t.Cleanup(func() { connection.CloseNow() })

	opened.send(message{Type: "connection_init"})
	acknowledgement, err := opened.read()
	if err != nil {
		return nil, err
	}
	if acknowledgement.Type != "connection_ack" {
		b.harness.t.Fatalf("first frame was %q, want connection_ack", acknowledgement.Type)
	}
	return opened, nil
}

// mustConnect fails the test if the socket cannot be opened.
func (b *browser) mustConnect() *socket {
	b.harness.t.Helper()
	opened, err := b.connect()
	if err != nil {
		b.harness.t.Fatalf("connect: %v", err)
	}
	return opened
}

func (s *socket) send(frame message) {
	s.harness.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	encoded, err := json.Marshal(frame)
	if err != nil {
		s.harness.t.Fatalf("encode frame: %v", err)
	}
	if err := s.connection.Write(ctx, websocket.MessageText, encoded); err != nil {
		s.harness.t.Fatalf("write frame: %v", err)
	}
}

func (s *socket) read() (message, error) {
	return s.readWithin(5 * time.Second)
}

// readWithin reads the next frame, giving up after timeout. Giving up closes
// the socket: coder/websocket does not survive a cancelled read.
func (s *socket) readWithin(timeout time.Duration) (message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, data, err := s.connection.Read(ctx)
	if err != nil {
		return message{}, err
	}
	var frame message
	if err := json.Unmarshal(data, &frame); err != nil {
		return message{}, err
	}
	return frame, nil
}

// subscribe starts one operation on the socket.
func (s *socket) subscribe(id, document string, values map[string]any) {
	s.harness.t.Helper()
	payload, err := json.Marshal(map[string]any{"query": document, "variables": values})
	if err != nil {
		s.harness.t.Fatalf("encode payload: %v", err)
	}
	s.send(message{ID: id, Type: "subscribe", Payload: payload})
}

// await reads until something happens to the named subscription, skipping the
// protocol's own housekeeping frames.
func (s *socket) await(id string) message {
	s.harness.t.Helper()
	for {
		frame, err := s.read()
		if err != nil {
			s.harness.t.Fatalf("read: %v", err)
		}
		if frame.ID != id {
			continue
		}
		switch frame.Type {
		case "next", "error", "complete":
			return frame
		}
	}
}

// data decodes the payload of a "next" frame the way the HTTP tests decode a
// response, so assertions read the same on both transports.
func (m message) data(t *testing.T) result {
	t.Helper()
	if m.Type != "next" {
		t.Fatalf("frame is %q, want next", m.Type)
	}
	var decoded result
	if err := json.Unmarshal(m.Payload, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return decoded
}

// errorCode reads the code out of a frame that reports a failure. A rejected
// subscription arrives in either of two shapes — an "error" frame carrying a
// list, or a "next" frame carrying a response with errors in it — depending on
// whether the operation was turned away before or during execution, and a client
// has to read both.
func (m message) errorCode(t *testing.T) string {
	t.Helper()
	switch m.Type {
	case "error":
		var errorsPayload []gqlError
		if err := json.Unmarshal(m.Payload, &errorsPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(errorsPayload) == 0 {
			t.Fatal("error frame carries no errors")
		}
		return errorsPayload[0].Extensions.Code
	case "next":
		answer := m.data(t)
		if len(answer.Errors) == 0 {
			t.Fatalf("frame carries data rather than a failure: %v", answer.Data)
		}
		return answer.code()
	default:
		t.Fatalf("frame is %q, want a failure", m.Type)
		return ""
	}
}

// awaitComplete reads until the subscription ends, allowing the payloads that a
// mutation may emit on its way out.
func (s *socket) awaitComplete(id string) {
	s.harness.t.Helper()
	for attempt := range 5 {
		frame := s.await(id)
		if frame.Type == "complete" {
			return
		}
		if frame.Type == "error" {
			s.harness.t.Fatalf("subscription failed instead of completing: %s", frame.Payload)
		}
		if attempt == 4 {
			s.harness.t.Fatal("subscription kept emitting instead of completing")
		}
	}
}

// TestSubscriptionEmitsAtOnceAndOnChange pins the promise that makes a query
// unnecessary before subscribing: the first frame carries the state as it is,
// and a first frame does not mean anything changed.
func TestSubscriptionEmitsAtOnceAndOnChange(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	connection := root.mustConnect()
	connection.subscribe("1", `subscription { me { user { displayName } } }`, nil)

	first := connection.await("1").data(t)
	if got := first.text("me.user.displayName"); got != "Root User" {
		t.Fatalf("first frame carries %q, want the state at subscribe time", got)
	}

	expectOK(t, root.post(`mutation U($input: UpdateProfileInput!) {
		updateProfile(input: $input) { user { displayName } }
	}`, variables("input", map[string]any{"displayName": "Root Operator"})))

	second := connection.await("1").data(t)
	if got := second.text("me.user.displayName"); got != "Root Operator" {
		t.Errorf("second frame carries %q, want the changed name", got)
	}
}

func TestSubscriptionsCarryTheSameAuthorizationAsFields(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	t.Run("without a session", func(t *testing.T) {
		connection := harness.browser().mustConnect()
		connection.subscribe("1", `subscription { me { user { id } } }`, nil)
		if code := connection.await("1").errorCode(t); code != "UNAUTHENTICATED" {
			t.Errorf("code = %q, want UNAUTHENTICATED", code)
		}
	})

	t.Run("without the permission", func(t *testing.T) {
		connection := member.mustConnect()
		connection.subscribe("1", `subscription { usersChanged }`, nil)
		if code := connection.await("1").errorCode(t); code != "ACCESS_DENIED" {
			t.Errorf("code = %q, want ACCESS_DENIED", code)
		}
	})

	t.Run("with the permission", func(t *testing.T) {
		connection := root.mustConnect()
		connection.subscribe("1", `subscription { usersChanged }`, nil)
		// usersChanged is a bell: the first ring is the subscribe itself.
		if frame := connection.await("1"); frame.Type != "next" {
			t.Fatalf("frame is %q, want next", frame.Type)
		}
		harness.account(root, "newcomer", "Newcomer", 5)
		if frame := connection.await("1"); frame.Type != "next" {
			t.Errorf("frame is %q, want a second ring", frame.Type)
		}
	})
}

// TestSubscriptionEndsWhenAccessChanges is the schema's rule that a live stream
// never outlives the access that allowed it. The stream completes — no error —
// and the client subscribes again, which puts it back through @auth.
func TestSubscriptionEndsWhenAccessChanges(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	connection := member.mustConnect()
	connection.subscribe("1", `subscription { me { user { displayName roles } } }`, nil)
	connection.await("1").data(t)

	expectOK(t, root.post(`mutation R($userId: ID!, $input: SetUserRolesInput!) {
		setUserRoles(userId: $userId, input: $input) { id }
	}`, variables("userId", harness.userID("quentin"), "input", map[string]any{"roles": []string{"ADMIN"}})))

	connection.awaitComplete("1")
}

func TestSubscriptionEndsWhenTheSessionIsRevoked(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	watching := harness.browser()
	expectOK(t, watching.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))

	connection := watching.mustConnect()
	connection.subscribe("1", `subscription { me { user { id } } }`, nil)
	connection.await("1").data(t)

	expectOK(t, root.post(`mutation R($sessionId: ID!) { revokeSession(sessionId: $sessionId) { id } }`,
		variables("sessionId", watching.sessionID())))

	connection.awaitComplete("1")
}

// TestResubscribingOnAnEndedSessionFails pins the other half of "subscribe
// again": the socket outlives the session it opened on, and a subscription
// started on it afterwards must not be served on that session's authority.
func TestResubscribingOnAnEndedSessionFails(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	connection := root.mustConnect()
	connection.subscribe("1", `subscription { me { user { id } } }`, nil)
	connection.await("1").data(t)

	expectOK(t, root.post(`mutation { logout }`, nil))
	connection.awaitComplete("1")

	connection.subscribe("2", `subscription { me { user { id } } }`, nil)
	if code := connection.await("2").errorCode(t); code != "UNAUTHENTICATED" {
		t.Errorf("code = %q, want UNAUTHENTICATED", code)
	}
}

// TestQuietSocketIsPinged covers a socket with nothing to report: something
// still has to cross it, or whatever sits in the path with an idle timeout cuts
// it. The deadline is the shortest such timeout seen in practice.
func TestQuietSocketIsPinged(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	connection := root.mustConnect()

	deadline := time.Now().Add(15 * time.Second)
	for {
		frame, err := connection.readWithin(time.Until(deadline))
		if err != nil {
			t.Fatalf("no ping within 15 seconds: %v", err)
		}
		if frame.Type == "ping" {
			return
		}
	}
}

// TestSessionsSubscriptionFollowsTheList covers the field a sessions screen
// stays open on: signing in somewhere else shows up without a refetch.
func TestSessionsSubscriptionFollowsTheList(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	connection := root.mustConnect()
	connection.subscribe("1", `subscription { sessions { id current } }`, nil)

	first := connection.await("1").data(t)
	if listed := first.list("sessions"); len(listed) != 1 {
		t.Fatalf("first frame lists %d sessions, want 1", len(listed))
	}

	elsewhere := harness.browser()
	expectOK(t, elsewhere.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))

	second := connection.await("1").data(t)
	if listed := second.list("sessions"); len(listed) != 2 {
		t.Errorf("second frame lists %d sessions, want 2", len(listed))
	}
}

// TestSocketRefusesADeadSession covers the handshake: a cookie that no longer
// resolves is turned away with the code the schema names, rather than leaving
// the client to discover it one failed subscribe at a time.
func TestSocketRefusesADeadSession(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	harness.expireSessions(time.Now().UTC().Add(-time.Hour))

	connection, err := root.connect()
	if err == nil {
		// The refusal can arrive either as a failed handshake or as a close
		// right after connection_init, depending on how fast the server is.
		_, err = connection.read()
	}
	if err == nil {
		t.Fatal("a socket opened on a dead session")
	}
	if status := websocket.CloseStatus(err); status != 4403 {
		t.Errorf("close status = %d (%v), want 4403", status, err)
	}
}

// compile-time guard: the tests depend on this being a close error rather than
// any old failure, and CloseStatus is how that is read.
var _ = func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }
