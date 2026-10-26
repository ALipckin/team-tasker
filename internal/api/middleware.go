package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"math"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type ctxKey int

const userIDKey ctxKey = iota

func userIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}

func (h *handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		userID, err := h.authService.ParseToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if lw, ok := w.(*loggingResponseWriter); ok {
			lw.userID = userID
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, userID)))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
	userID int64
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}

// requestLogger пишет структурированный лог по каждому запросу.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)

		lw := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lw, r)

		attrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", lw.status,
			"size", lw.size,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", clientIP(r),
		}
		if lw.userID != 0 {
			attrs = append(attrs, "user_id", lw.userID)
		}

		level := slog.LevelInfo
		if lw.status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "http request", attrs...)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered", "panic", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter — token bucket на каждый IP клиента.
type rateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	if burst <= 0 {
		burst = int(math.Ceil(rps))
	}

	rl := &rateLimiter{
		rate:    rps,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
	go rl.cleanup(time.Minute)

	return rl
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
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

// cleanup удаляет давно неактивные bucket'ы, чтобы карта не росла бесконечно.
func (rl *rateLimiter) cleanup(every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for key, b := range rl.buckets {
			if rl.now().Sub(b.last) > every {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}
