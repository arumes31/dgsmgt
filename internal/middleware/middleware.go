package middleware

import (
	"context"
	"dgsmgt/internal/auth"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type contextKey string

const ClaimsKey contextKey = "claims"

func IPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, err := peerAddress(r.RemoteAddr)
			if err == nil && addressInPrefixes(peer, trustedProxies) {
				if client, ok := forwardedClientAddress(r, peer, trustedProxies); ok {
					r.RemoteAddr = net.JoinHostPort(client.String(), "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func peerAddress(remoteAddr string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return netip.ParseAddr(strings.Trim(host, "[]"))
}

func addressInPrefixes(address netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func forwardedClientAddress(r *http.Request, peer netip.Addr, trusted []netip.Prefix) (netip.Addr, bool) {
	chain := []netip.Addr{peer}
	for _, raw := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			address, err := netip.ParseAddr(raw)
			if err != nil {
				return netip.Addr{}, false
			}
			chain = append([]netip.Addr{address}, chain...)
		}
	}
	for index := len(chain) - 1; index >= 0; index-- {
		if !addressInPrefixes(chain[index], trusted) {
			return chain[index], true
		}
	}
	return netip.Addr{}, false
}

// LoggingMiddleware uses zap for structured logging
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)

			logger.Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				cookie, err := r.Cookie("token")
				if err != nil {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				authHeader = cookie.Value
			} else {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					authHeader = parts[1]
				}
			}

			claims, err := auth.VerifyToken(authHeader, secret)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(ClaimsKey).(*auth.Claims)
		if !ok || !claims.IsAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware implements strict IP-based rate limiting
func RateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu          sync.Mutex
		clients     = make(map[string]*client)
		lastCleanup = time.Now()
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}

			mu.Lock()
			now := time.Now()
			if now.Sub(lastCleanup) >= time.Minute {
				for clientIP, c := range clients {
					if now.Sub(c.lastSeen) > 5*time.Minute {
						delete(clients, clientIP)
					}
				}
				lastCleanup = now
			}
			if _, found := clients[ip]; !found {
				clients[ip] = &client{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
			}
			clients[ip].lastSeen = now
			limiter := clients[ip].limiter
			mu.Unlock()

			if !limiter.Allow() {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// PayloadLimitMiddleware limits incoming HTTP payload sizes
func PayloadLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
