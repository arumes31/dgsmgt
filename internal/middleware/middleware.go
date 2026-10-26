package middleware

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

type contextKey string

const ClaimsKey contextKey = "claims"

// DefaultTrustedProxies are used when TRUST_PROXY is enabled without TRUSTED_PROXIES:
// loopback and private networks, where a local reverse proxy usually lives.
var DefaultTrustedProxies = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"fc00::/7",
}

// ParseCIDRs parses a list of CIDRs or single IP addresses.
func ParseCIDRs(values []string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !strings.Contains(v, "/") {
			if ip := net.ParseIP(v); ip != nil && ip.To4() != nil {
				v += "/32"
			} else {
				v += "/128"
			}
		}
		_, n, err := net.ParseCIDR(v)
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	return nets, nil
}

func containsIP(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IPMiddleware rewrites RemoteAddr from forwarding headers, but only when the
// direct peer is a trusted proxy. An empty list disables header parsing.
func IPMiddleware(trustedProxies []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(trustedProxies) > 0 {
				if ip := forwardedClientIP(r, trustedProxies); ip != "" {
					r.RemoteAddr = net.JoinHostPort(ip, "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func forwardedClientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := net.ParseIP(RemoteIP(r))
	if peer == nil || !containsIP(trusted, peer) {
		return ""
	}
	if cfIP := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); cfIP != nil {
		return cfIP.String()
	}
	xff := r.Header.Values("X-Forwarded-For")
	if len(xff) == 0 {
		return ""
	}
	// Walk from the right and return the first address that is not a trusted proxy
	ips := strings.Split(strings.Join(xff, ","), ",")
	for i := len(ips) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(ips[i]))
		if ip == nil {
			return ""
		}
		if i == 0 || !containsIP(trusted, ip) {
			return ip.String()
		}
	}
	return ""
}

// RemoteIP returns the host part of r.RemoteAddr without the port.
func RemoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
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

// AuthMiddleware verifies the bearer token (or, for WebSocket upgrades, a
// single-use ticket) and reloads the user from the database, so deleted users,
// demoted admins and revoked tokens are rejected immediately.
func AuthMiddleware(secret string, database *gorm.DB, tickets *TicketStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var claims *auth.Claims

			authHeader := r.Header.Get("Authorization")
			switch {
			case authHeader != "":
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) != 2 || parts[0] != "Bearer" {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				c, err := auth.VerifyToken(parts[1], secret)
				if err != nil {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				claims = c
			case tickets != nil && isWebSocketUpgrade(r) && r.URL.Query().Get("ticket") != "":
				c, ok := tickets.Redeem(r.URL.Query().Get("ticket"))
				if !ok {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				claims = c
			default:
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			var user models.User
			if err := database.First(&user, claims.UserID).Error; err != nil || user.TokenVersion != claims.TokenVersion {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			claims.Username = user.Username
			claims.IsAdmin = user.IsAdmin
			claims.MustChangePassword = user.MustChangePassword

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// PasswordChangeMiddleware blocks every request except the allowed paths while
// the user still has to replace an initial or default password.
func PasswordChangeMiddleware(allowedPaths ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedPaths))
	for _, p := range allowedPaths {
		allowed[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ClaimsKey).(*auth.Claims)
			if ok && claims.MustChangePassword && !allowed[r.URL.Path] {
				http.Error(w, "Password change required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
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

// TicketStore issues short-lived, single-use tickets that authenticate
// WebSocket connections, so long-lived JWTs never end up in URLs.
type TicketStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	tickets map[string]wsTicket
}

type wsTicket struct {
	claims  auth.Claims
	expires time.Time
}

func NewTicketStore(ttl time.Duration) *TicketStore {
	return &TicketStore{ttl: ttl, tickets: make(map[string]wsTicket)}
}

// Issue creates a ticket for the given claims.
func (s *TicketStore) Issue(claims *auth.Claims) (string, error) {
	id, err := auth.RandomString(32)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.tickets {
		if now.After(t.expires) {
			delete(s.tickets, k)
		}
	}
	s.tickets[id] = wsTicket{claims: *claims, expires: now.Add(s.ttl)}
	return id, nil
}

// Redeem consumes a ticket and returns its claims if it is still valid.
func (s *TicketStore) Redeem(id string) (*auth.Claims, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok {
		return nil, false
	}
	delete(s.tickets, id)
	if time.Now().After(t.expires) {
		return nil, false
	}
	c := t.claims
	return &c, true
}

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
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			for ip, c := range clients {
				if time.Since(c.lastSeen) > 5*time.Minute {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := RemoteIP(r)

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
