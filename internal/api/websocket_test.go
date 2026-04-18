package api

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type mockWSClient struct {
	mockClient
	statsFunc func(ctx context.Context, containerID string) (io.ReadCloser, error)
	logsFunc  func(ctx context.Context, containerID string, tail string) (io.ReadCloser, error)
}

func (m *mockWSClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	if m.statsFunc != nil {
		rc, err := m.statsFunc(ctx, containerID)
		return container.StatsResponseReader{Body: rc}, err
	}
	return container.StatsResponseReader{Body: io.NopCloser(strings.NewReader(`{"id":"test"}`))}, nil
}

func (m *mockWSClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.logsFunc != nil {
		return m.logsFunc(ctx, containerID, options.Tail)
	}
	// Return some data with docker header
	header := []byte{1, 0, 0, 0, 0, 0, 0, 5}
	payload := []byte("hello")
	return io.NopCloser(strings.NewReader(string(append(header, payload...)))), nil
}

func TestMetricsHandlerWS(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockWSClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "ws-id"}
	db.Create(&server)
	
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"id": "ws-id"})
		claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
		ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
		api.MetricsHandler(w, r.WithContext(ctx))
	}))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer ws.Close()

	_, message, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !strings.Contains(string(message), "test") {
		t.Errorf("Unexpected message: %s", string(message))
	}
}

func TestLogsHandlerWS(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockWSClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "ws-logs-id"}
	db.Create(&server)
	
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"id": "ws-logs-id"})
		claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
		ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
		api.LogsHandler(w, r.WithContext(ctx))
	}))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer ws.Close()

	_, message, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !strings.Contains(string(message), "hello") {
		t.Errorf("Unexpected message: %s", string(message))
	}
}
