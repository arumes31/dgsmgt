package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPMiddleware(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "1.2.3.4:0" {
			t.Errorf("Expected RemoteAddr 1.2.3.4:0, got %s", r.RemoteAddr)
		}
	})
	handlerToTest := IPMiddleware(true)(nextHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	w := httptest.NewRecorder()

	handlerToTest.ServeHTTP(w, req)
}
