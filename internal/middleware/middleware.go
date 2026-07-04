package middleware

import (
	"context"
	"crypto/rand"
	"dgsmgt/internal/auth"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type contextKey string

const ClaimsKey contextKey = "claims"
const RequestIDKey contextKey = "request_id"

// IPMiddleware normalizes the client IP when running behind a reverse proxy
// or Cloudflare (CF-Connecting-IP takes precedence over X-Forwarded-For).
func IPMiddleware(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if trustProxy {
				if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
					r.RemoteAddr = net.JoinHostPort(cfIP, "0")
				} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					ips := strings.Split(xff, ",")
					r.RemoteAddr = net.JoinHostPort(strings.TrimSpace(ips[0]), "0")
				} else if xri := r.Header.Get("X-Real-IP"); xri != "" {
					r.RemoteAddr = net.JoinHostPort(strings.TrimSpace(xri), "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP extracts the bare IP from RemoteAddr.
func ClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RequestIDMiddleware attaches a request ID to the context and response.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestID returns the request ID from context (empty if absent).
func RequestID(r *http.Request) string {
	if v, ok := r.Context().Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
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
				zap.String("request_id", RequestID(r)),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

// RecoverMiddleware converts panics into 500s instead of dropping connections.
func RecoverMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						zap.Any("panic", rec),
						zap.String("path", r.URL.Path),
						zap.String("request_id", RequestID(r)))
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// WebSocket clients can't set headers: allow token query param there.
				if r.Header.Get("Upgrade") == "websocket" {
					authHeader = r.URL.Query().Get("token")
				}
				if authHeader == "" {
					cookie, err := r.Cookie("token")
					if err != nil {
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
					authHeader = cookie.Value
				}
			} else {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					authHeader = parts[1]
				}
			}

			claims, err := auth.VerifyToken(authHeader, secret)
			if err != nil || claims.Pending2FA {
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

// RootMiddleware restricts a route to the superadmin/root user
// (permanent deletes, panel settings, node management).
func RootMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(ClaimsKey).(*auth.Claims)
		if !ok || !claims.IsRoot {
			http.Error(w, "Forbidden: requires superadmin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var cleanupInterval = time.Minute
var clientTimeout = 5 * time.Minute
var RateLimitShutdown = make(chan struct{})

// RateLimitMiddleware implements strict IP-based rate limiting
func RateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	// Cleanup old clients periodically
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				for ip, c := range clients {
					if time.Since(c.lastSeen) > clientTimeout {
						delete(clients, ip)
					}
				}
				mu.Unlock()
			case <-RateLimitShutdown:
				return
			}
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r)

			mu.Lock()
			if _, found := clients[ip]; !found {
				clients[ip] = &client{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
			}
			clients[ip].lastSeen = time.Now()
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
