// Package middleware provides HTTP middleware for the openclaw proxy, including rate limiting.
package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

type RateLimiter struct {
	mu             sync.Mutex
	buckets        map[string]*bucket
	rate           float64 // tokens per second
	capacity       float64
	TrustedProxies []string // trusted proxy IPs; when set, X-Forwarded-For/X-Real-IP are used for client IP
}

func NewRateLimiter(ctx context.Context, perMinute int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     float64(perMinute) / 60.0,
		capacity: float64(perMinute),
	}
	go rl.cleanup(ctx)
	return rl
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		rl.buckets[ip] = &bucket{tokens: rl.capacity - 1, lastCheck: now}
		return true
	}

	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastCheck = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanup evicts stale entries every 5 minutes to prevent unbounded memory growth.
// Stops when the context is cancelled.
func (rl *RateLimiter) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, b := range rl.buckets {
				if b.lastCheck.Before(cutoff) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// clientIP extracts the client IP from the request. If the direct peer is a
// trusted proxy, X-Forwarded-For or X-Real-IP headers are used instead of
// RemoteAddr.
func (rl *RateLimiter) clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}

	if len(rl.TrustedProxies) > 0 && rl.isTrustedProxy(ip) {
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return realIP
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Use the left-most (original client) IP.
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}

	return ip
}

func (rl *RateLimiter) isTrustedProxy(ip string) bool {
	for _, trusted := range rl.TrustedProxies {
		if trusted == ip {
			return true
		}
	}
	return false
}

func (rl *RateLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.clientIP(r)
		if !rl.Allow(ip) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
