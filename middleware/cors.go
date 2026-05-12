package middleware

import (
	"net/http"
	"sync"
	"time"
)

type visitor struct {
	count    int
	windowStart time.Time
}

var (
	visitors = make(map[string]*visitor)
	mu       sync.Mutex
)

// RateLimit allows max 10 requests per minute per IP.
func RateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		mu.Lock()
		v, exists := visitors[ip]
		if !exists || time.Since(v.windowStart) > time.Minute {
			visitors[ip] = &visitor{count: 1, windowStart: time.Now()}
			mu.Unlock()
			next(w, r)
			return
		}

		if v.count >= 10 {
			mu.Unlock()
			http.Error(w, `{"error":"rate limit exceeded — max 10 requests per minute"}`, http.StatusTooManyRequests)
			return
		}

		v.count++
		mu.Unlock()
		next(w, r)
	}
}
