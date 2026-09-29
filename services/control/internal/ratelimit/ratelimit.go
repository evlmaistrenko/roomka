// Package ratelimit holds the counters that keep one client from taking more
// than its share: token buckets keyed by whatever is being limited (an address,
// an account, a username), and caps on how many of something one key may hold at
// once. Everything lives in memory, which is right for a single process and
// means a restart forgets every count.
package ratelimit

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rate is a token bucket: Burst attempts at once, and one more every Every.
type Rate struct {
	Burst int
	Every time.Duration
}

func (r Rate) valid() bool { return r.Burst > 0 && r.Every > 0 }

// sweepEvery is how often a Keyed drops the buckets that have refilled. A full
// bucket behaves exactly like a missing one, so dropping it loses nothing, and
// without it every address that ever called would be remembered forever.
const sweepEvery = time.Minute

// Keyed is a set of token buckets, one per key, all with the same Rate.
type Keyed struct {
	rate      Rate
	mutex     sync.Mutex
	buckets   map[string]*rate.Limiter
	lastSweep time.Time
}

// NewKeyed builds an empty set of buckets.
func NewKeyed(bucketRate Rate) *Keyed {
	return &Keyed{rate: bucketRate, buckets: map[string]*rate.Limiter{}, lastSweep: time.Now()}
}

// Allow spends one attempt for key. When none is left it spends nothing and
// reports how long until one is.
func (k *Keyed) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	limiter := k.bucket(key, now)
	reservation := limiter.ReserveN(now, 1)
	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)
		return false, delay
	}
	return true, 0
}

// Check reports whether key has an attempt left, without spending it. Paired
// with Spend it counts only what went wrong: check before trying, spend after
// failing.
func (k *Keyed) Check(key string) (bool, time.Duration) {
	now := time.Now()
	tokens := k.bucket(key, now).TokensAt(now)
	if tokens >= 1 {
		return true, 0
	}
	return false, time.Duration(math.Ceil((1 - tokens) * float64(k.rate.Every)))
}

// Spend uses up one attempt for key, going into debt if there is none left, so
// that failures past the limit keep pushing the next attempt further out.
func (k *Keyed) Spend(key string) {
	now := time.Now()
	k.bucket(key, now).ReserveN(now, 1)
}

func (k *Keyed) bucket(key string, now time.Time) *rate.Limiter {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	if now.Sub(k.lastSweep) >= sweepEvery {
		for existing, limiter := range k.buckets {
			if limiter.TokensAt(now) >= float64(k.rate.Burst) {
				delete(k.buckets, existing)
			}
		}
		k.lastSweep = now
	}
	limiter, ok := k.buckets[key]
	if !ok {
		limiter = rate.NewLimiter(rate.Every(k.rate.Every), k.rate.Burst)
		k.buckets[key] = limiter
	}
	return limiter
}

// Single is one token bucket, for something that is already per-client, such as
// one WebSocket connection.
type Single struct {
	rate    Rate
	limiter *rate.Limiter
}

// NewSingle builds a full bucket.
func NewSingle(bucketRate Rate) *Single {
	return &Single{rate: bucketRate, limiter: rate.NewLimiter(rate.Every(bucketRate.Every), bucketRate.Burst)}
}

// Allow spends one attempt, or reports how long until one is left.
func (s *Single) Allow() (bool, time.Duration) {
	now := time.Now()
	reservation := s.limiter.ReserveN(now, 1)
	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)
		return false, delay
	}
	return true, 0
}

// Concurrent caps how many of something one key may hold at the same time.
type Concurrent struct {
	limit  int
	mutex  sync.Mutex
	counts map[string]int
}

// NewConcurrent builds a cap of limit per key.
func NewConcurrent(limit int) *Concurrent {
	return &Concurrent{limit: limit, counts: map[string]int{}}
}

// Acquire takes one slot for key, or reports that all of them are taken. Every
// successful Acquire must be matched by a Release.
func (c *Concurrent) Acquire(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.counts[key] >= c.limit {
		return false
	}
	c.counts[key]++
	return true
}

// Release gives back a slot taken by Acquire.
func (c *Concurrent) Release(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.counts[key] <= 1 {
		delete(c.counts, key)
		return
	}
	c.counts[key]--
}
