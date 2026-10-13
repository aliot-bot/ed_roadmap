package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type limiter struct {
	mu     sync.Mutex
	limit  int
	period time.Duration
	visits map[string][]time.Time
}

func newLimiter(limit int, period time.Duration) *limiter {
	return &limiter{
		limit:  limit,
		period: period,
		visits: map[string][]time.Time{},
	}
}

func (a *API) loginLimited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
		if err != nil || len(body) > 1<<20 {
			writeError(w, 400, "invalid request", nil)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Login string `json:"login"`
		}
		_ = json.Unmarshal(body, &request)

		if !a.authLimiter.allow(loginLimitKey(r, request.Login)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, 429, "too many requests", nil)
			return
		}
		next(w, r)
	}
}

func loginLimitKey(r *http.Request, login string) string {
	candidate := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if candidate == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			candidate = host
		} else {
			candidate = r.RemoteAddr
		}
	}
	ip := net.ParseIP(candidate)
	normalizedIP := "unknown"
	if ip != nil {
		normalizedIP = ip.String()
	}
	return normalizedIP + ":" + strings.ToLower(strings.TrimSpace(login))
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.period)
	old := l.visits[key]
	keep := old[:0]
	for _, t := range old {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}

	if len(keep) >= l.limit {
		l.visits[key] = keep
		return false
	}
	l.visits[key] = append(keep, now)
	return true
}
