package middleware

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestIPMiddleware(t *testing.T) {
	// X-Forwarded-For
	mw := IPMiddleware(true)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "1.2.3.4:0" {
			t.Errorf("Expected IP 1.2.3.4:0, got %s", r.RemoteAddr)
		}
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	// CF-Connecting-IP
	next2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "2.3.4.5:0" {
			t.Errorf("Expected IP 2.3.4.5:0, got %s", r.RemoteAddr)
		}
	})
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("CF-Connecting-IP", "2.3.4.5")
	w2 := httptest.NewRecorder()
	mw(next2).ServeHTTP(w2, req2)

	// TrustProxy = false
	mw3 := IPMiddleware(false)
	origAddr := "127.0.0.1:1234"
	next3 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != origAddr {
			t.Errorf("Expected original address %s, got %s", origAddr, r.RemoteAddr)
		}
	})
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = origAddr
	req3.Header.Set("X-Forwarded-For", "1.1.1.1")
	w3 := httptest.NewRecorder()
	mw3(next3).ServeHTTP(w3, req3)
}

func TestLoggingMiddleware(t *testing.T) {
	logger := zap.NewNop()
	mw := LoggingMiddleware(logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	mw(next).ServeHTTP(w, req)
}

func TestAuthMiddleware(t *testing.T) {
	secret := "test-secret"
	mw := AuthMiddleware(secret)

	token, _ := auth.GenerateToken(&models.User{Username: "test"}, secret, time.Hour)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(ClaimsKey).(*auth.Claims)
		if claims.Username != "test" {
			t.Errorf("Expected username test, got %s", claims.Username)
		}
	})

	// Bearer Token
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	// Cookie
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	w = httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Invalid Bearer Token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w = httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for invalid bearer, got %d", w.Code)
	}

	// Empty Authorization
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for empty auth, got %d", w.Code)
	}
}

func TestAdminMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Is Admin
	claims := &auth.Claims{IsAdmin: true}
	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, claims)
	w := httptest.NewRecorder()
	AdminMiddleware(next).ServeHTTP(w, req.WithContext(ctx))
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Is Not Admin
	claims = &auth.Claims{IsAdmin: false}
	req = httptest.NewRequest("GET", "/", nil)
	ctx = context.WithValue(req.Context(), ClaimsKey, claims)
	w = httptest.NewRecorder()
	AdminMiddleware(next).ServeHTTP(w, req.WithContext(ctx))
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	mw := RateLimitMiddleware(1, 1) // 1 request per second, burst 1
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"

	// First request - OK
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Second request immediately - Rate limited
	w = httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429, got %d", w.Code)
	}

	// Malformed IP request
	reqMalformed := httptest.NewRequest("GET", "/", nil)
	reqMalformed.RemoteAddr = "malformed-ip-without-port"
	w = httptest.NewRecorder()
	mw(next).ServeHTTP(w, reqMalformed)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for new malformed IP, got %d", w.Code)
	}
}

func TestPayloadLimitMiddleware(t *testing.T) {
	mw := PayloadLimitMiddleware(5)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err == nil {
			t.Error("Expected error for oversized payload")
		}
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader("too long payload"))
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)
}

func TestRateLimitMiddlewareCleanup(t *testing.T) {
	cleanupInterval = 10 * time.Millisecond
	clientTimeout = 1 * time.Millisecond
	mw := RateLimitMiddleware(1, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"

	// Add a client
	mw(next).ServeHTTP(httptest.NewRecorder(), req)

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Test shutdown
	select {
	case RateLimitShutdown <- struct{}{}:
	case <-time.After(100 * time.Millisecond):
	}
}
