package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestProxyRejectsUnmanagedContainer(t *testing.T) {
	mock := &mockDockerClient{
		inspectFunc: func(context.Context, string) (container.InspectResponse, error) {
			return container.InspectResponse{
				ID:     "unmanaged",
				Config: &container.Config{Labels: map[string]string{}},
			}, nil
		},
	}
	handler := NewProxyHandler(NewServiceWithClient(mock))
	request := httptest.NewRequest(http.MethodPost, "/v1/start/unmanaged", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestProxyRejectsUnlistedImage(t *testing.T) {
	policy, err := NewCreationPolicy(
		[]string{"registry.example/game@sha256:" + strings.Repeat("a", 64)},
		[]string{"/srv/dgsmgt"},
	)
	if err != nil {
		t.Fatalf("NewCreationPolicy() error = %v", err)
	}
	handler := NewProxyHandler(NewServiceWithClientAndPolicy(&mockDockerClient{}, policy))
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/containers",
		strings.NewReader(`{"name":"bad-image","image":"attacker/image:latest"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
