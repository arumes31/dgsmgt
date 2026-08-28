package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInternalErrorDoesNotLeakDetails(t *testing.T) {
	response := httptest.NewRecorder()
	InternalError(response, "database password: super-secret")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "super-secret") {
		t.Fatal("InternalError() leaked implementation details")
	}
}
