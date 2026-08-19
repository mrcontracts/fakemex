package apiv1

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"log/slog"
)

type requestContextKey string

const requestIDKey requestContextKey = "requestID"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		r = r.WithContext(r.Context())
		r.Header.Set("X-Request-ID", requestID)
		r = withValue(r, requestIDKey, requestID)
		next.ServeHTTP(w, r)
	})
}

func withValue(r *http.Request, key requestContextKey, value string) *http.Request {
	t := r.WithContext(context.WithValue(r.Context(), key, value))
	return t
}

func ContextRequestID(r *http.Request) string {
	if v := r.Context().Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func SecurityMiddleware(origin string, localOnly bool, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Exact origin enforcement on state-changing endpoints and websocket upgrade.
			streamRequest := r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/stream")
			stateChanging := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
			if stateChanging || streamRequest {
				o := r.Header.Get("Origin")
				if o == "" || (o != origin && !(localOnly && equivalentLoopbackOrigins(o, origin))) {
					logger.Warn("origin mismatch", "expected", origin, "got", o, "path", r.URL.Path)
					http.Error(w, "origin not allowed", http.StatusForbidden)
					return
				}
			}
			if localOnly {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					host = r.RemoteAddr
				}
				if host != "" {
					split := strings.TrimSpace(host)
					if split != "localhost" && split != "127.0.0.1" && split != "::1" {
						ip, err := netip.ParseAddr(host)
						if err != nil || !ip.IsLoopback() {
							http.Error(w, "local-only service", http.StatusForbidden)
							return
						}
					}
				}
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/stream") {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/") {
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Referrer-Policy", "no-referrer")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func equivalentLoopbackOrigins(left, right string) bool {
	l, lerr := url.Parse(left)
	r, rerr := url.Parse(right)
	if lerr != nil || rerr != nil || l.Scheme != r.Scheme || l.Port() != r.Port() {
		return false
	}
	isLoopback := func(host string) bool {
		if strings.EqualFold(host, "localhost") {
			return true
		}
		ip, err := netip.ParseAddr(host)
		return err == nil && ip.IsLoopback()
	}
	return isLoopback(l.Hostname()) && isLoopback(r.Hostname())
}

type loggingResponseWriter struct {
	status int
	w      http.ResponseWriter
}

func (l *loggingResponseWriter) Header() http.Header {
	return l.w.Header()
}

func (l *loggingResponseWriter) WriteHeader(code int) {
	l.status = code
	l.w.WriteHeader(code)
}

func (l *loggingResponseWriter) Write(data []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	return l.w.Write(data)
}

func (l *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := l.w.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (l *loggingResponseWriter) Flush() {
	flusher, ok := l.w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (l *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := l.w.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &loggingResponseWriter{w: w, status: http.StatusOK}
			next.ServeHTTP(wrapped, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration_ms", time.Since(start).Milliseconds())
		})
	}
}

func withWriteJSON(w http.ResponseWriter, code int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
