package ratelimit

import (
	"errors"
	"time"
)

// Policy is every limit the server enforces, in one place so the numbers can be
// read side by side. The proxy in front of the server is expected to be a little
// stricter still (see the Caddy note in CONTRIBUTING.md); these are the floor
// that holds even when something reaches the server directly.
type Policy struct {
	// Requests is every HTTP request from one address, bar the health check.
	Requests Rate

	// PasswordAttemptsPerAddress counts every password check (login, stepUp,
	// changePassword) from one address, successful or not.
	PasswordAttemptsPerAddress Rate
	// PasswordFailuresPerAccount counts wrong passwords against one account,
	// from anywhere. Only failures count, so the owner signing in is never the
	// one who uses it up.
	PasswordFailuresPerAccount Rate

	// ResetRequestsPerAddress and ResetRequestsPerUsername limit
	// requestPasswordReset, which anyone may call for any username.
	ResetRequestsPerAddress  Rate
	ResetRequestsPerUsername Rate
	// ResetInterval is how long an issued reset secret is left alone before a
	// new request may replace it. Without it, anybody could keep cancelling the
	// owner's link by asking for a new one.
	ResetInterval time.Duration

	// SocketsPerAddress caps open subscription WebSockets from one address.
	SocketsPerAddress int
	// SocketOperations is the operations one socket may start.
	SocketOperations Rate
	// SubscriptionsPerSocket caps subscriptions running on one socket at once.
	SubscriptionsPerSocket int
}

// DefaultPolicy is the policy the server runs with.
func DefaultPolicy() Policy {
	return Policy{
		Requests: Rate{Burst: 100, Every: 50 * time.Millisecond}, // 20 a second

		PasswordAttemptsPerAddress: Rate{Burst: 20, Every: 6 * time.Second}, // 10 a minute
		PasswordFailuresPerAccount: Rate{Burst: 10, Every: time.Minute},

		ResetRequestsPerAddress:  Rate{Burst: 5, Every: time.Minute},
		ResetRequestsPerUsername: Rate{Burst: 3, Every: 10 * time.Minute},
		ResetInterval:            time.Minute,

		SocketsPerAddress:      20,
		SocketOperations:       Rate{Burst: 30, Every: 100 * time.Millisecond}, // 10 a second
		SubscriptionsPerSocket: 50,
	}
}

// Validate refuses a policy with a limit left at zero. Zero is not "unlimited"
// here: a zero bucket admits nothing, so a Policy built field by field that
// forgot one would quietly shut something off.
func (p Policy) Validate() error {
	rates := []Rate{
		p.Requests,
		p.PasswordAttemptsPerAddress, p.PasswordFailuresPerAccount,
		p.ResetRequestsPerAddress, p.ResetRequestsPerUsername,
		p.SocketOperations,
	}
	for _, bucketRate := range rates {
		if !bucketRate.valid() {
			return errors.New("rate limit policy has a rate left at zero")
		}
	}
	if p.ResetInterval <= 0 || p.SocketsPerAddress <= 0 || p.SubscriptionsPerSocket <= 0 {
		return errors.New("rate limit policy has a limit left at zero")
	}
	return nil
}
