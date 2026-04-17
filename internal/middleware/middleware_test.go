package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dgsmgt/internal/auth"
)

func TestMiddleware(t *testing.T) {
	password := "testpass"
	authenticator := auth.NewAuthenticator(password, false)

	t.Run("AuthMiddleware denied", func(t *testing.T) {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		handlerToTest := AuthMiddleware(authenticator)(nextHandler)

		req := httptest.NewRequest("GET", "/api/status", nil)
		w := httptest.NewRecorder()

		handlerToTest.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("AuthMiddleware allowed", func(t *testing.T) {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		handlerToTest := AuthMiddleware(authenticator)(nextHandler)

		req := httptest.NewRequest("GET", "/api/status", nil)
		req.AddCookie(&http.Cookie{Name: authenticator.SessionCookie, Value: password})
		w := httptest.NewRecorder()

		handlerToTest.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("IPMiddleware with X-Forwarded-For", func(t *testing.T) {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !testing.Short() {
				// RemoteAddr will have "0" as port because of net.JoinHostPort
				if r.RemoteAddr != "1.2.3.4:0" {
					t.Errorf("Expected RemoteAddr 1.2.3.4:0, got %s", r.RemoteAddr)
				}
			}
		})
		handlerToTest := IPMiddleware(true)(nextHandler)

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		w := httptest.NewRecorder()

		handlerToTest.ServeHTTP(w, req)
	})

	t.Run("IPMiddleware with CF-Connecting-IP", func(t *testing.T) {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !testing.Short() {
				if r.RemoteAddr != "9.9.9.9:0" {
					t.Errorf("Expected RemoteAddr 9.9.9.9:0, got %s", r.RemoteAddr)
				}
			}
		})
		handlerToTest := IPMiddleware(true)(nextHandler)

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "9.9.9.9")
		w := httptest.NewRecorder()

		handlerToTest.ServeHTTP(w, req)
	})
}
