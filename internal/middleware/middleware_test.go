package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestIPMiddleware(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "1.2.3.4:0" {
			t.Errorf("Expected RemoteAddr 1.2.3.4:0, got %s", r.RemoteAddr)
		}
	})
	handlerToTest := IPMiddleware([]netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("5.6.7.0/24"),
	})(nextHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	w := httptest.NewRecorder()

	handlerToTest.ServeHTTP(w, req)
}

func TestIPMiddlewareIgnoresUntrustedPeer(t *testing.T) {
	nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "198.51.100.7:1234" {
			t.Errorf("RemoteAddr = %q, want original peer", r.RemoteAddr)
		}
	})
	handler := IPMiddleware([]netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")})(nextHandler)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.7:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
