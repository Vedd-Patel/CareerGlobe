package server

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// limiter is an in-memory token bucket per key (a client IP or a user). It
// costs nothing to run and is enough for one API process behind the tunnel.
type limiter struct {
	rate  float64 // tokens added per second
	burst float64
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

const maxBuckets = 100_000

func newLimiter(perMinute, burst int) *limiter {
	return &limiter{rate: float64(perMinute) / 60, burst: float64(burst), now: time.Now, buckets: map[string]*bucket{}}
}

// allow takes one token for key. When none is left it reports how long to wait.
func (l *limiter) allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxBuckets {
			l.sweep(now)
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
}

// sweep drops buckets that have refilled completely (idle clients); if every
// client is still active the map is reset rather than left to grow.
func (l *limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
	if len(l.buckets) >= maxBuckets {
		l.buckets = map[string]*bucket{}
	}
}

// rateError is a 429 with a Retry-After header.
type rateError struct{ wait time.Duration }

func (e *rateError) Error() string { return "rate_limited" }

func tooMany(wait time.Duration) error { return &rateError{wait} }

// throttled limits every request by client IP (RATE_LIMIT_PER_MINUTE). The
// health check is exempt so uptime monitors never trip it.
func (s *Server) throttled(next http.Handler) http.Handler {
	if s.limit == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		key := "?"
		if ip := s.clientIP(r); ip != nil {
			key = ip.String()
		}
		if ok, wait := s.limit.allow(key); !ok {
			handler(func(http.ResponseWriter, *http.Request) error { return tooMany(wait) }).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// perUser limits one expensive endpoint per member (resume parsing, reports).
func (s *Server) perUser(l *limiter, h handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if ok, wait := l.allow(strconv.FormatInt(account(r).UserID, 10)); !ok {
			return tooMany(wait)
		}
		return h(w, r)
	}
}
