package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

func RateLimit(limit rate.Limit, burst int) func(http.Handler) http.Handler {
	store := &limiterStore{
		limit:     limit,
		burst:     burst,
		clients:   make(map[string]*clientLimiter),
		maxIdle:   10 * time.Minute,
		nextSweep: time.Now().Add(time.Minute),
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isRateLimitExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if !store.allow(clientKey(r)) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate_limited","message":"too many requests"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type limiterStore struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	clients   map[string]*clientLimiter
	maxIdle   time.Duration
	nextSweep time.Time
}

func (s *limiterStore) allow(key string) bool {
	now := time.Now()

	s.mu.Lock()
	if now.After(s.nextSweep) {
		s.sweep(now)
		s.nextSweep = now.Add(time.Minute)
	}

	entry, ok := s.clients[key]
	if !ok {
		entry = &clientLimiter{limiter: rate.NewLimiter(s.limit, s.burst), lastSeen: now}
		s.clients[key] = entry
	} else {
		entry.lastSeen = now
	}
	allowed := entry.limiter.Allow()
	s.mu.Unlock()

	return allowed
}

func (s *limiterStore) sweep(now time.Time) {
	for key, entry := range s.clients {
		if now.Sub(entry.lastSeen) > s.maxIdle {
			delete(s.clients, key)
		}
	}
}

func isRateLimitExempt(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

func clientKey(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			candidate := strings.TrimSpace(parts[0])
			if candidate != "" {
				return candidate
			}
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}

	return "unknown"
}
