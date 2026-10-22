package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dgsmgt/internal/auth"
)

func TestIPMiddleware(t *testing.T) {
	trusted, err := ParseCIDRs(DefaultTrustedProxies)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		cf         string
		trusted    bool
		want       string
	}{
		{"trusted proxy, right-most untrusted XFF entry wins", "10.0.0.2:1234", "1.2.3.4, 5.6.7.8", "", true, "5.6.7.8:0"},
		{"trusted proxy chain is skipped", "10.0.0.2:1234", "1.2.3.4, 192.168.1.10", "", true, "1.2.3.4:0"},
		{"trusted proxy with CF header", "127.0.0.1:1234", "", "9.9.9.9", true, "9.9.9.9:0"},
		{"untrusted peer cannot spoof XFF", "203.0.113.7:1234", "1.2.3.4", "", true, "203.0.113.7:1234"},
		{"untrusted peer cannot spoof CF header", "203.0.113.7:1234", "", "1.2.3.4", true, "203.0.113.7:1234"},
		{"proxy support disabled", "10.0.0.2:1234", "1.2.3.4", "", false, "10.0.0.2:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			})
			nets := trusted
			if !tt.trusted {
				nets = nil
			}
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.cf != "" {
				req.Header.Set("CF-Connecting-IP", tt.cf)
			}
			IPMiddleware(nets)(next).ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.want {
				t.Errorf("RemoteAddr = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTicketStore(t *testing.T) {
	store := NewTicketStore(time.Minute)
	id, err := store.Issue(&auth.Claims{UserID: 7})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.Redeem(id)
	if !ok || c.UserID != 7 {
		t.Fatalf("expected valid ticket for user 7, got %v %v", c, ok)
	}
	if _, ok := store.Redeem(id); ok {
		t.Fatal("ticket must be single-use")
	}

	expired := NewTicketStore(-time.Second)
	id, _ = expired.Issue(&auth.Claims{UserID: 7})
	if _, ok := expired.Redeem(id); ok {
		t.Fatal("expired ticket must be rejected")
	}
}

func TestPasswordChangeMiddleware(t *testing.T) {
	h := PasswordChangeMiddleware("/api/me/password")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	claims := &auth.Claims{UserID: 1, MustChangePassword: true}

	for path, want := range map[string]int{"/api/me/password": http.StatusOK, "/api/my-servers": http.StatusForbidden} {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(context.WithValue(req.Context(), ClaimsKey, claims))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("%s: got %d, want %d", path, w.Code, want)
		}
	}
}
