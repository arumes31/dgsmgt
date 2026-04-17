package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticator(t *testing.T) {
	password := "testpass"
	auth := NewAuthenticator(password, false)

	t.Run("Login success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"password": password})
		req := httptest.NewRequest("POST", "/login", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		auth.LoginHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		cookie := w.Result().Cookies()[0]
		if cookie.Name != auth.SessionCookie || cookie.Value != password {
			t.Error("Cookie not set correctly")
		}
	})

	t.Run("Login failure", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		req := httptest.NewRequest("POST", "/login", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		auth.LoginHandler(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("IsAuthenticated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		if auth.IsAuthenticated(req) {
			t.Error("Should not be authenticated without cookie")
		}

		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: password})
		if !auth.IsAuthenticated(req) {
			t.Error("Should be authenticated with correct cookie")
		}
	})

	t.Run("Logout", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/logout", nil)
		w := httptest.NewRecorder()

		auth.LogoutHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		cookie := w.Result().Cookies()[0]
		if cookie.MaxAge != -1 {
			t.Error("Cookie should be expired on logout")
		}
	})
}
