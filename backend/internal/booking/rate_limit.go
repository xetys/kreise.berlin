package booking

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter is a simple in-memory per-IP token bucket suitable for
// single-process deployments. For multi-replica setups behind a real
// ingress, prefer ingress-level rate limiting.
type IPRateLimiter struct {
	r        rate.Limit
	burst    int
	mu       sync.Mutex
	limiters map[string]*entry
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPRateLimiter: r requests/second sustained, burst initial allowance.
func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	return &IPRateLimiter{r: r, burst: burst, limiters: map[string]*entry{}}
}

func (l *IPRateLimiter) get(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.limiters[ip]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.r, l.burst)}
		l.limiters[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// gc periodically drops entries unused for >1 hour. Call once at startup.
func (l *IPRateLimiter) gcLoop() {
	t := time.NewTicker(10 * time.Minute)
	for range t.C {
		cutoff := time.Now().Add(-time.Hour)
		l.mu.Lock()
		for ip, e := range l.limiters {
			if e.lastSeen.Before(cutoff) {
				delete(l.limiters, ip)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware returns a handler that enforces this limiter. The IP is taken
// from r.RemoteAddr (chi middleware.RealIP should run before this so
// X-Forwarded-For is honored when running behind a trusted proxy).
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.get(ip).Allow() {
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many requests; try again in a moment")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// StartGC kicks off the limiter's gc loop in its own goroutine. Call once at startup.
func (l *IPRateLimiter) StartGC() {
	go l.gcLoop()
}
