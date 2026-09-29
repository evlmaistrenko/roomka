package ratelimit

import (
	"testing"
	"time"
)

func TestKeyedAllowsTheBurstThenWaits(t *testing.T) {
	keyed := NewKeyed(Rate{Burst: 2, Every: time.Hour})

	for attempt := range 2 {
		if allowed, _ := keyed.Allow("a"); !allowed {
			t.Fatalf("attempt %d within the burst was refused", attempt+1)
		}
	}
	allowed, wait := keyed.Allow("a")
	if allowed || wait <= 0 {
		t.Errorf("attempt past the burst = %v after %v, want refused with a wait", allowed, wait)
	}
	if allowed, _ := keyed.Allow("b"); !allowed {
		t.Error("another key was refused: keys must not share a bucket")
	}
}

// Check does not spend, so only what Spend records counts.
func TestCheckAndSpendCountOnlyWhatIsSpent(t *testing.T) {
	keyed := NewKeyed(Rate{Burst: 1, Every: time.Hour})

	for range 3 {
		if allowed, _ := keyed.Check("a"); !allowed {
			t.Fatal("check spent an attempt")
		}
	}
	keyed.Spend("a")
	if allowed, wait := keyed.Check("a"); allowed || wait <= 0 {
		t.Errorf("check after spending the burst = %v after %v, want refused with a wait", allowed, wait)
	}
}

func TestConcurrentCapsAndReleases(t *testing.T) {
	concurrent := NewConcurrent(1)

	if !concurrent.Acquire("a") {
		t.Fatal("first slot refused")
	}
	if concurrent.Acquire("a") {
		t.Error("second slot granted past the cap")
	}
	concurrent.Release("a")
	if !concurrent.Acquire("a") {
		t.Error("slot not given back on release")
	}
}

func TestPolicyRefusesAZeroLimit(t *testing.T) {
	if err := DefaultPolicy().Validate(); err != nil {
		t.Fatalf("default policy: %v", err)
	}
	policy := DefaultPolicy()
	policy.PasswordFailuresPerAccount = Rate{}
	if policy.Validate() == nil {
		t.Error("a policy with a zero rate validated")
	}
}
