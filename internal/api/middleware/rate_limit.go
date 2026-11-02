package middleware

import (
	"math"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a token bucket for each client IP.
type RateLimiter struct {
	mu          sync.Mutex
	rate        float64
	burst       float64
	buckets     map[string]*bucket
	now         func() time.Time
	nextCleanup time.Time
	writeError  ErrorWriter
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimiter(rps float64, burst int, writeError ErrorWriter) *RateLimiter {
	if burst <= 0 {
		burst = int(math.Ceil(rps))
	}
	limiter := &RateLimiter{
		rate:       rps,
		burst:      float64(burst),
		buckets:    make(map[string]*bucket),
		now:        time.Now,
		writeError: writeError,
	}
	return limiter
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	if !now.Before(rl.nextCleanup) {
		for key, b := range rl.buckets {
			if now.Sub(b.last) > time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.nextCleanup = now.Add(time.Minute)
	}
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	b.tokens = math.Min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(ClientIP(r)) {
			w.Header().Set("Retry-After", "1")
			rl.writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
