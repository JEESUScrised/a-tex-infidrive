package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LoginLimiter ограничивает попытки входа по IP.
type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	window   time.Duration
	max      int
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	if max < 1 {
		max = 10
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	return &LoginLimiter{attempts: map[string][]time.Time{}, window: window, max: max}
}

// Allow проверяет лимит. Удачные входы не должны «сжигать» квоту — вызывайте Fail после ошибки.
func (l *LoginLimiter) Allow(r *http.Request) bool {
	ip := clientIP(r)
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	arr := l.attempts[ip]
	kept := arr[:0]
	for _, t := range arr {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	l.attempts[ip] = kept
	return len(kept) < l.max
}

// Fail фиксирует неудачную попытку входа.
func (l *LoginLimiter) Fail(r *http.Request) {
	ip := clientIP(r)
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	arr := l.attempts[ip]
	kept := arr[:0]
	for _, t := range arr {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	l.attempts[ip] = append(kept, now)
}

func clientIP(r *http.Request) string {
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
