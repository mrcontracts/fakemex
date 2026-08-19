package rate

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	visitors  map[string]*visitor
	mu        sync.Mutex
	rateLimit rate.Limit
	burst     int
	expiry    time.Duration
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewLimiter(r rate.Limit, burst int) *Limiter {
	return &Limiter{
		visitors:  make(map[string]*visitor),
		rateLimit: r,
		burst:     burst,
		expiry:    3 * time.Minute,
	}
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr == "" {
			next.ServeHTTP(w, r)
			return
		}
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		v := l.getVisitor(ip)
		if !v.limiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) getVisitor(ip string) *visitor {
	l.mu.Lock()
	defer l.mu.Unlock()
	if v, ok := l.visitors[ip]; ok {
		v.lastSeen = time.Now()
		return v
	}
	v := &visitor{
		limiter:  rate.NewLimiter(l.rateLimit, l.burst),
		lastSeen: time.Now(),
	}
	l.visitors[ip] = v
	return v
}

func (l *Limiter) cleanupLoop(stop <-chan struct{}) {
	t := time.NewTicker(l.expiry)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.mu.Lock()
			for k, v := range l.visitors {
				if time.Since(v.lastSeen) > l.expiry {
					delete(l.visitors, k)
				}
			}
			l.mu.Unlock()
		case <-stop:
			return
		}
	}
}
