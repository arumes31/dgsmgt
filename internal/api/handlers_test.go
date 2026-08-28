package api

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dgsmgt/internal/docker"

	"github.com/moby/moby/api/types/container"
	"go.uber.org/zap"
)

type mockClient struct {
}

func (m *mockClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	return container.InspectResponse{
		ID:     "1234567890abcdef",
		State:  &container.State{Status: container.StateRunning},
		Config: &container.Config{Image: "img", Labels: map[string]string{"com.dgsmgt.managed": "true"}},
	}, nil
}

func (m *mockClient) ContainerStart(ctx context.Context, containerID string) error {
	return nil
}
func (m *mockClient) ContainerStop(ctx context.Context, containerID string) error {
	return nil
}
func (m *mockClient) ContainerRestart(ctx context.Context, containerID string) error {
	return nil
}
func (m *mockClient) ContainerLogs(ctx context.Context, containerID, tail string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("log line")), nil
}
func (m *mockClient) ContainerCreate(ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig) (string, error) {
	return "new-id", nil
}
func (m *mockClient) ContainerRemove(ctx context.Context, containerID string, force bool) error {
	return nil
}
func (m *mockClient) ContainerList(ctx context.Context) ([]container.Summary, error) {
	return []container.Summary{}, nil
}
func (m *mockClient) ImagePull(ctx context.Context, ref string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockClient) ContainerStats(ctx context.Context, containerID string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("{}")), nil
}

func TestAPIHandlers(t *testing.T) {
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	logger, _ := zap.NewDevelopment()
	api := NewAPI(svc, nil, "secret", nil, logger)

	if api == nil {
		t.Fatal("Failed to create API")
	}
}

func TestWebSocketOriginPolicy(t *testing.T) {
	logger := zap.NewNop()

	t.Run("same origin by default", func(t *testing.T) {
		api := NewAPI(nil, nil, "secret", nil, logger)
		req := httptest.NewRequest("GET", "https://example.com/api/logs/id", nil)
		req.Header.Set("Origin", "https://example.com")
		if !api.checkOrigin(req) {
			t.Fatal("checkOrigin() rejected the same origin")
		}
	})

	t.Run("cross origin denied by default", func(t *testing.T) {
		api := NewAPI(nil, nil, "secret", nil, logger)
		req := httptest.NewRequest("GET", "https://example.com/api/logs/id", nil)
		req.Header.Set("Origin", "https://attacker.example")
		if api.checkOrigin(req) {
			t.Fatal("checkOrigin() accepted a cross origin request")
		}
	})

	t.Run("explicit origin", func(t *testing.T) {
		api := NewAPI(nil, nil, "secret", []string{"https://console.example"}, logger)
		req := httptest.NewRequest("GET", "https://internal/api/logs/id", nil)
		req.Header.Set("Origin", "https://console.example")
		if !api.checkOrigin(req) {
			t.Fatal("checkOrigin() rejected an explicitly allowed origin")
		}
	})
}

func TestLoginAttemptTrackingIsConcurrentAndExpires(t *testing.T) {
	api := NewAPI(nil, nil, "secret", nil, zap.NewNop())
	now := time.Now()
	key := loginKey{clientIP: "192.0.2.1", username: "user"}

	var group sync.WaitGroup
	for range 10 {
		group.Add(1)
		go func() {
			defer group.Done()
			api.recordLoginFailure(key, now)
		}()
	}
	group.Wait()

	if !api.loginBlocked(key, now) {
		t.Fatal("loginBlocked() = false after repeated failures")
	}
	if api.loginBlocked(key, now.Add(loginLockout)) {
		t.Fatal("loginBlocked() retained an expired lockout")
	}
}
