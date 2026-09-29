package apitest

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/ratelimit"
)

// tightened starts a server whose limits are the defaults with some made small
// enough to reach in a test.
func tightened(t *testing.T, adjust func(*ratelimit.Policy)) *harness {
	t.Helper()
	return newHarness(t, func(configuration *config.Config) { adjust(&configuration.Limits) })
}

// never is a refill slow enough that nothing comes back while a test runs.
const never = time.Hour

func login(b *browser, username, password string) result {
	return b.post(loginMutation, variables("input", map[string]any{
		"username": username,
		"password": password,
	}))
}

func expectRateLimited(t *testing.T, answer result) {
	t.Helper()
	expect(t, answer, "RATE_LIMITED")
	if len(answer.Errors) > 0 && answer.Errors[0].Extensions.RetryAfterSeconds <= 0 {
		t.Errorf("retryAfterSeconds = %d, want a positive wait", answer.Errors[0].Extensions.RetryAfterSeconds)
	}
}

// Wrong passwords against one account run out, and then even the right one is
// turned away until the budget refills: that is what makes guessing slow.
func TestWrongPasswordsLockTheAccountForAWhile(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.PasswordFailuresPerAccount = ratelimit.Rate{Burst: 2, Every: never}
	})
	harness.signUp()
	guesser := harness.browser()

	expect(t, login(guesser, "root", "wrong one"), "UNAUTHENTICATED")
	expect(t, login(guesser, "root", "wrong two"), "UNAUTHENTICATED")
	expectRateLimited(t, login(guesser, "root", rootPassword))
}

// A username with no account behind it runs out exactly like one with: any
// difference would say which usernames exist.
func TestUnknownUsernamesAreLimitedLikeKnownOnes(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.PasswordFailuresPerAccount = ratelimit.Rate{Burst: 2, Every: never}
	})
	guesser := harness.browser()

	expect(t, login(guesser, "nobody", "wrong one"), "UNAUTHENTICATED")
	expect(t, login(guesser, "nobody", "wrong two"), "UNAUTHENTICATED")
	expectRateLimited(t, login(guesser, "nobody", "wrong three"))
}

// Only failures count against an account, so its owner signing in over and over
// never locks it.
func TestSigningInDoesNotUseUpTheAccountBudget(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.PasswordFailuresPerAccount = ratelimit.Rate{Burst: 1, Every: never}
	})
	harness.signUp()

	for range 3 {
		expectOK(t, login(harness.browser(), "root", rootPassword))
	}
}

// One address guessing across many accounts runs out too.
func TestPasswordAttemptsFromOneAddressRunOut(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.PasswordAttemptsPerAddress = ratelimit.Rate{Burst: 3, Every: never}
	})
	guesser := harness.browser()

	for _, username := range []string{"alice", "bob", "carol"} {
		expect(t, login(guesser, username, "a guess"), "UNAUTHENTICATED")
	}
	expectRateLimited(t, login(guesser, "dave", "a guess"))
}

const requestPasswordResetMutation = `mutation Reset($username: String!) {
	requestPasswordReset(username: $username)
}`

func TestPasswordResetRequestsPerUsernameRunOut(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.ResetRequestsPerUsername = ratelimit.Rate{Burst: 2, Every: never}
	})
	requester := harness.browser()

	for range 2 {
		expectOK(t, requester.post(requestPasswordResetMutation, variables("username", "nobody")))
	}
	expectRateLimited(t, requester.post(requestPasswordResetMutation, variables("username", "nobody")))
}

// A second request soon after the first answers the same, but leaves the first
// secret in place: otherwise anybody could keep cancelling the owner's link.
func TestPasswordResetIsNotReissuedWithinTheInterval(t *testing.T) {
	harness := newHarness(t)
	harness.signUp()
	requester := harness.browser()

	expectOK(t, requester.post(requestPasswordResetMutation, variables("username", "root")))
	first := harness.secretFor("root")
	expectOK(t, requester.post(requestPasswordResetMutation, variables("username", "root")))

	if printed := strings.Count(harness.console.text(), `password reset for "root"`); printed != 1 {
		t.Errorf("secrets printed = %d, want the first one only", printed)
	}
	if harness.secretFor("root") != first {
		t.Error("the first secret was replaced within the interval")
	}
}

// Every request from one address counts, bar the health check.
func TestRequestsFromOneAddressRunOut(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.Requests = ratelimit.Rate{Burst: 2, Every: never}
	})

	for range 2 {
		if status, _, _ := harness.get("/openapi.json"); status != http.StatusOK {
			t.Fatalf("GET /openapi.json within the limit = %d", status)
		}
	}
	response, err := harness.server.Client().Get(harness.url + "/openapi.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") == "" {
		t.Errorf("GET past the limit = %d, Retry-After %q; want 429 with a wait",
			response.StatusCode, response.Header.Get("Retry-After"))
	}
	if status, _, _ := harness.get("/health"); status != http.StatusOK {
		t.Errorf("GET /health past the limit = %d, want it exempt", status)
	}
}

func TestOversizedRequestBodiesAreRefused(t *testing.T) {
	harness := newHarness(t)

	body := `{"query":"{ version }","variables":{"padding":"` + strings.Repeat("x", 2<<20) + `"}}`
	response, err := harness.server.Client().Post(harness.url+"/graphql", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("POST of 2 MiB = %d, want 413", response.StatusCode)
	}
}

func TestSocketsFromOneAddressAreCapped(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) { policy.SocketsPerAddress = 1 })
	browser := harness.browser()

	browser.mustConnect()
	if _, err := browser.connect(); err == nil {
		t.Error("a second socket from the same address opened, want it refused")
	}
}

const meSubscription = `subscription { me { user { id } } }`

func TestSubscriptionsPerSocketAreCapped(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) { policy.SubscriptionsPerSocket = 1 })
	socket := harness.signUp().mustConnect()

	socket.subscribe("1", meSubscription, nil)
	if frame := socket.await("1"); frame.Type != "next" {
		t.Fatalf("first subscription answered %q, want next", frame.Type)
	}
	socket.subscribe("2", meSubscription, nil)
	if code := socket.await("2").errorCode(t); code != "LIMIT_REACHED" {
		t.Errorf("second subscription = %s, want LIMIT_REACHED", code)
	}
}

func TestOperationsPerSocketRunOut(t *testing.T) {
	harness := tightened(t, func(policy *ratelimit.Policy) {
		policy.SocketOperations = ratelimit.Rate{Burst: 2, Every: never}
	})
	socket := harness.signUp().mustConnect()

	for _, id := range []string{"1", "2"} {
		socket.subscribe(id, `{ version }`, nil)
		if frame := socket.await(id); frame.Type != "next" {
			t.Fatalf("operation %s answered %q, want next", id, frame.Type)
		}
	}
	socket.subscribe("3", `{ version }`, nil)
	if code := socket.await("3").errorCode(t); code != "RATE_LIMITED" {
		t.Errorf("third operation = %s, want RATE_LIMITED", code)
	}
}
