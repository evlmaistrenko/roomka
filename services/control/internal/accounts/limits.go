package accounts

import (
	"context"
	"time"

	"control/internal/identity"
	"control/internal/ratelimit"
	"control/internal/schema"
)

// limits are the rate limits that need to know what a call is about — which
// account a password is for, which username a reset is asked for — and so live
// here rather than in front of the whole server with the per-request one.
type limits struct {
	passwordAttempts *ratelimit.Keyed // by caller address
	passwordFailures *ratelimit.Keyed // by username
	resetRequests    *ratelimit.Keyed // by caller address
	resetsByUsername *ratelimit.Keyed // by username
	resetInterval    time.Duration
}

func newLimits(policy ratelimit.Policy) *limits {
	return &limits{
		passwordAttempts: ratelimit.NewKeyed(policy.PasswordAttemptsPerAddress),
		passwordFailures: ratelimit.NewKeyed(policy.PasswordFailuresPerAccount),
		resetRequests:    ratelimit.NewKeyed(policy.ResetRequestsPerAddress),
		resetsByUsername: ratelimit.NewKeyed(policy.ResetRequestsPerUsername),
		resetInterval:    policy.ResetInterval,
	}
}

// callerAddress is the address the per-address limits count by.
func callerAddress(ctx context.Context) string {
	return requestOrigin(ctx).IPAddress
}

// provePassword is the one place a password is checked against a stored hash,
// so every way of guessing one — login, stepUp, changePassword — draws on the
// same two budgets: attempts from the caller's address, and failures against the
// account, wherever they come from.
//
// The account is named by username, not id, so that a username with no account
// behind it is limited exactly like one with: a different answer, or a faster
// one, would say which usernames exist. For the same reason an unknown user is
// checked against an empty hash, which costs as much as a real check.
func (s *Service) provePassword(ctx context.Context, username, password, passwordHash string) error {
	if allowed, wait := s.limits.passwordAttempts.Allow(callerAddress(ctx)); !allowed {
		return schema.RateLimited(wait)
	}
	if allowed, wait := s.limits.passwordFailures.Check(username); !allowed {
		return schema.RateLimited(wait)
	}
	if err := identity.VerifyPassword(password, passwordHash); err != nil {
		s.limits.passwordFailures.Spend(username)
		return schema.ErrInvalidCredentials
	}
	return nil
}

// admitResetRequest applies requestPasswordReset's limits that do not depend on
// whether the username exists, so failing them says nothing about it.
func (s *Service) admitResetRequest(ctx context.Context, username string) error {
	if allowed, wait := s.limits.resetRequests.Allow(callerAddress(ctx)); !allowed {
		return schema.RateLimited(wait)
	}
	if allowed, wait := s.limits.resetsByUsername.Allow(username); !allowed {
		return schema.RateLimited(wait)
	}
	return nil
}
