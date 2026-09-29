// Package apitest drives the API the way a client does: real HTTP, real cookies,
// real GraphQL documents, against the server main builds. It holds no code of
// its own — only tests — which is why it is a directory rather than a file
// beside the resolvers.
//
// The tests are deliberately end-to-end rather than direct calls on the resolver
// methods. Almost everything being tested here happens outside those methods —
// in the @auth directive, in the @constraint directive, in the middleware that
// turns a cookie into a principal, in the error presenter that gives a failure
// its code. A test that called the methods directly would be testing the half of
// the system that was never in doubt.
package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"control/graph"
	"control/internal/accounts"
	"control/internal/config"
	"control/internal/database"
	"control/internal/events"
	"control/internal/identity"
	"control/internal/model"
	"control/internal/ratelimit"
	"control/internal/rbac"
	"control/internal/server"
)

// harness is one running server with its own database.
type harness struct {
	t       *testing.T
	url     string
	store   *database.Database
	console *console
	server  *httptest.Server
}

// The server runs over TLS because the session cookie is Secure and a cookie jar
// will not send a Secure cookie over plain HTTP — the test has to meet the same
// condition a browser does.
// newHarness starts a server; an option may change its configuration before it
// starts, for the tests that need it to be set up differently.
func newHarness(t *testing.T, options ...func(*config.Config)) *harness {
	t.Helper()

	// os.MkdirTemp with best-effort cleanup rather than t.TempDir: on Windows
	// the sqlite WAL/SHM files can still be locked when RemoveAll runs, and
	// t.TempDir would fail the test on that cleanup error.
	dir, err := os.MkdirTemp("", "roomka-graph-test-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	store, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	roles, err := rbac.Load(graph.Schema(), store)
	if err != nil {
		t.Fatalf("load roles: %v", err)
	}

	// The listener exists before the server starts, so the public URL — which
	// the Origin check compares against — is known in time to be configured.
	testServer := httptest.NewUnstartedServer(nil)
	publicURL := "https://" + testServer.Listener.Addr().String()

	configuration := config.Config{
		DatabasePath: filepath.Join(dir, "test.db"),
		PublicURL:    publicURL,
		Version:      "test-version",
		Limits:       ratelimit.DefaultPolicy(),
	}
	for _, option := range options {
		option(&configuration)
	}

	resolver := &graph.Resolver{
		Accounts: accounts.New(configuration, store, events.NewBus(), roles),
	}
	handler, err := server.New(configuration, store, resolver)
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	testServer.Config.Handler = handler
	testServer.StartTLS()

	if testServer.URL != publicURL {
		t.Fatalf("server URL %s is not the configured public URL %s", testServer.URL, publicURL)
	}

	t.Cleanup(func() {
		testServer.Close()
		store.Close()
		_ = os.RemoveAll(dir)
	})

	return &harness{
		t:       t,
		url:     testServer.URL,
		store:   store,
		console: captureConsole(t),
		server:  testServer,
	}
}

// browser is one client with its own cookie jar — which is what makes it one
// device, as far as the server is concerned.
type browser struct {
	harness *harness
	client  *http.Client
}

func (h *harness) browser() *browser {
	h.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		h.t.Fatalf("cookie jar: %v", err)
	}
	return &browser{harness: h, client: &http.Client{
		// The test server's own client is configured to trust its certificate;
		// every browser is that transport with a cookie jar of its own.
		Transport: h.server.Client().Transport,
		Jar:       jar,
	}}
}

// post sends one GraphQL operation and decodes the response.
func (b *browser) post(query string, variables map[string]any) result {
	b.harness.t.Helper()
	return b.postWithHeaders(query, variables, nil)
}

func (b *browser) postWithHeaders(query string, variables map[string]any, headers map[string]string) result {
	b.harness.t.Helper()

	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		b.harness.t.Fatalf("encode request: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, b.harness.url+"/graphql", bytes.NewReader(body))
	if err != nil {
		b.harness.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	// A browser sends Origin on every same-origin POST, and the server checks
	// it on every call that carries the cookie.
	request.Header.Set("Origin", b.harness.url)
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := b.client.Do(request)
	if err != nil {
		b.harness.t.Fatalf("post: %v", err)
	}
	defer response.Body.Close()

	decoded := result{status: response.StatusCode, cookies: response.Cookies()}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		b.harness.t.Fatalf("decode response (status %d): %v", response.StatusCode, err)
	}
	return decoded
}

// result is a decoded GraphQL response.
type result struct {
	Data   map[string]any `json:"data"`
	Errors []gqlError     `json:"errors"`

	status  int
	cookies []*http.Cookie
}

type gqlError struct {
	Message    string `json:"message"`
	Path       []any  `json:"path"`
	Extensions struct {
		Code      string   `json:"code"`
		InputPath []string `json:"inputPath"`
		Reason    string   `json:"reason"`
		// RetryAfterSeconds is set on RATE_LIMITED.
		RetryAfterSeconds int `json:"retryAfterSeconds"`
	} `json:"extensions"`
}

// code is the error code of the first error, or "" when the call succeeded. A
// test asserting on it is asserting on the contract, not on prose.
func (r result) code() string {
	if len(r.Errors) == 0 {
		return ""
	}
	return r.Errors[0].Extensions.Code
}

func (r result) reason() string {
	if len(r.Errors) == 0 {
		return ""
	}
	return r.Errors[0].Extensions.Reason
}

func (r result) inputPath() []string {
	if len(r.Errors) == 0 {
		return nil
	}
	return r.Errors[0].Extensions.InputPath
}

func (r result) message() string {
	if len(r.Errors) == 0 {
		return ""
	}
	return r.Errors[0].Message
}

// at walks the response data by dotted path: at("me.user.username").
func (r result) at(path string) any {
	var current any = r.Data
	for _, step := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[step]
	}
	return current
}

func (r result) text(path string) string {
	value, _ := r.at(path).(string)
	return value
}

func (r result) number(path string) float64 {
	value, _ := r.at(path).(float64)
	return value
}

func (r result) list(path string) []any {
	value, _ := r.at(path).([]any)
	return value
}

// cookie returns the named cookie from the response, if the server set one.
func (r result) cookie(name string) *http.Cookie {
	for _, cookie := range r.cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// console captures what the server prints. Password reset secrets are delivered
// by printing them, so reading them back is not a test convenience — it is the
// only channel the schema gives them.
type console struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func captureConsole(t *testing.T) *console {
	t.Helper()
	captured := &console{}
	previous := log.Writer()
	log.SetOutput(captured)
	t.Cleanup(func() { log.SetOutput(previous) })
	return captured
}

func (c *console) Write(written []byte) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.buffer.Write(written)
}

func (c *console) text() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.buffer.String()
}

// The console prints a link to the UI's page, with the secret in its fragment.
var secretPattern = regexp.MustCompile(`password reset for "([^"]+)", valid until \S+: \S+/set-password#(\S+)`)

// secretFor returns the most recent reset secret printed for a username.
func (h *harness) secretFor(username string) string {
	h.t.Helper()
	matches := secretPattern.FindAllStringSubmatch(h.console.text(), -1)
	for index := len(matches) - 1; index >= 0; index-- {
		if matches[index][1] == username {
			return matches[index][2]
		}
	}
	h.t.Fatalf("no password reset secret was printed for %q; console was:\n%s", username, h.console.text())
	return ""
}

// The states a client cannot reach on its own, because reaching them means
// waiting: a lapsed step-up, an expired session. Every one of them is written
// through bun, like the code under test, so that a time a test sets and a time
// the server sets are stored in the same format.

// lapseStepUp puts every session back to "has not proved a password recently".
func (h *harness) lapseStepUp() {
	h.t.Helper()
	h.setOnSessions(nil, "step_up_expires_at = NULL")
}

// expireSessions moves every session's expiry.
func (h *harness) expireSessions(at time.Time) {
	h.t.Helper()
	h.setOnSessions(nil, "expires_at = ?", at)
}

// expireSession moves one session's expiry.
func (h *harness) expireSession(sessionID string, at time.Time) {
	h.t.Helper()
	h.setOnSessions(&sessionID, "expires_at = ?", at)
}

func (h *harness) setOnSessions(sessionID *string, assignment string, arguments ...any) {
	h.t.Helper()
	query := h.store.NewUpdate().
		Model((*model.Session)(nil)).
		Set(assignment, arguments...)
	if sessionID != nil {
		query = query.Where("id = ?", *sessionID)
	} else {
		query = query.Where("1 = 1")
	}
	if _, err := query.Exec(context.Background()); err != nil {
		h.t.Fatalf("set %s: %v", assignment, err)
	}
}

// execute runs a statement the query builder has no business expressing — the
// test that breaks the schema on purpose.
func (h *harness) execute(statement string) {
	h.t.Helper()
	if _, err := h.store.Exec(statement); err != nil {
		h.t.Fatalf("execute %q: %v", statement, err)
	}
}

// expect fails the test unless the call failed with exactly this code.
func expect(t *testing.T, answer result, code string) {
	t.Helper()
	if answer.code() != code {
		t.Fatalf("code = %q (message %q), want %q", answer.code(), answer.message(), code)
	}
}

// expectOK fails the test if the call returned any error at all.
func expectOK(t *testing.T, answer result) {
	t.Helper()
	if len(answer.Errors) > 0 {
		t.Fatalf("unexpected error: %s (code %s)", answer.message(), answer.code())
	}
}

func variables(pairs ...any) map[string]any {
	built := map[string]any{}
	for index := 0; index+1 < len(pairs); index += 2 {
		name, ok := pairs[index].(string)
		if !ok {
			panic(fmt.Sprintf("variable name %v is not a string", pairs[index]))
		}
		built[name] = pairs[index+1]
	}
	return built
}

// The documents the tests share. They are written out in full, the way a client
// would send them, so that what is being exercised is the schema rather than a
// helper's idea of it.
const (
	setupMutation = `mutation Setup($input: SetupInput!) {
		setup(input: $input) { user { id username displayName rank roles permissions passwordSetAt } session { id current } }
	}`
	loginMutation = `mutation Login($input: LoginInput!) {
		login(input: $input) { user { id username } session { id current stepUpExpiresAt } permissions }
	}`
	meQuery = `query Me {
		me { user { id username displayName rank roles directPermissions permissions } permissions
			session { id current expiresAt } preferences { key value } }
	}`
	createUserMutation = `mutation CreateUser($input: CreateUserInput!) {
		createUser(input: $input) { id username rank roles permissions passwordSetAt }
	}`
	setPasswordMutation = `mutation SetPassword($secret: String!, $input: SetPasswordInput!) {
		setPassword(secret: $secret, input: $input) { user { id username passwordSetAt } }
	}`
	sessionsQuery = `query Sessions($filter: SessionFilterInput, $sortBy: SessionSort) {
		sessions(filter: $filter, sortBy: $sortBy) { id current endedAt lastUserAgent }
	}`
)

// Passwords used by the tests. They are ordinary phrases: long, varied, and not
// built out of the account they protect, which is all the server asks.
const (
	rootPassword   = "quiet lantern by the river"
	memberPassword = "velvet harbour marmalade"
)

// signUp completes setup and returns the root user's browser.
func (h *harness) signUp() *browser {
	h.t.Helper()
	root := h.browser()
	answer := root.post(setupMutation, variables("input", map[string]any{
		"username":    "root",
		"displayName": "Root User",
		"password":    rootPassword,
	}))
	expectOK(h.t, answer)
	return root
}

// member creates an account through createUser, claims it with the printed
// secret, and returns the browser that now holds its session — the whole path a
// new person takes, since there is no other way to get one.
func (h *harness) member(root *browser, username string, rank int, roles []string, permissions []string) *browser {
	h.t.Helper()
	answer := root.post(createUserMutation, variables("input", map[string]any{
		"username":    username,
		"displayName": strings.ToUpper(username[:1]) + username[1:],
		"rank":        rank,
		"roles":       roles,
		"permissions": permissions,
	}))
	expectOK(h.t, answer)

	claimed := h.browser()
	answer = claimed.post(setPasswordMutation, variables(
		"secret", h.secretFor(username),
		"input", map[string]any{"password": memberPassword},
	))
	expectOK(h.t, answer)
	return claimed
}

// sessionCookie is the session cookie this browser is currently holding, if any.
func (b *browser) sessionCookie() *http.Cookie {
	b.harness.t.Helper()
	address, err := url.Parse(b.harness.url)
	if err != nil {
		b.harness.t.Fatalf("parse server url: %v", err)
	}
	for _, cookie := range b.client.Jar.Cookies(address) {
		if cookie.Name == identity.SessionCookieName {
			return cookie
		}
	}
	return nil
}

// holds makes this browser present someone else's cookie, which is what a
// stolen or stale one amounts to.
func (b *browser) holds(cookie *http.Cookie) {
	b.harness.t.Helper()
	address, err := url.Parse(b.harness.url)
	if err != nil {
		b.harness.t.Fatalf("parse server url: %v", err)
	}
	b.client.Jar.SetCookies(address, []*http.Cookie{cookie})
}

// userID is a user's id, read straight from the store. A test naming somebody
// in an argument is not testing how it found them.
func (h *harness) userID(username string) string {
	h.t.Helper()
	var id int64
	if err := h.store.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id); err != nil {
		h.t.Fatalf("no user %q: %v", username, err)
	}
	return strconv.FormatInt(id, 10)
}

// postRaw sends an operation and returns the status and body as they came, for
// the cases where the server answers before GraphQL is reached at all.
func (b *browser) postRaw(query string, headers map[string]string) (int, string) {
	b.harness.t.Helper()

	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		b.harness.t.Fatalf("encode request: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, b.harness.url+"/graphql", bytes.NewReader(body))
	if err != nil {
		b.harness.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := b.client.Do(request)
	if err != nil {
		b.harness.t.Fatalf("post: %v", err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		b.harness.t.Fatalf("read body: %v", err)
	}
	return response.StatusCode, string(answer)
}
