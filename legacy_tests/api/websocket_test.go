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
	"time"

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

type infiniteReader struct{}
func (i infiniteReader) Read(p []byte) (n int, err error) {
	return copy(p, []byte("abcdefghijklmnop")), nil
}
func (i infiniteReader) Close() error { return nil }

type failingReader struct{}
func (f failingReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF // returning generic error to trigger condition
}
func (f failingReader) Close() error { return nil }

func TestMetricsHandlerWS_WriteFail(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockWSClient{
		statsFunc: func(ctx context.Context, containerID string) (io.ReadCloser, error) {
			return infiniteReader{}, nil
		},
	}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "ws-write-id"}
	db.Create(&server)
	
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"id": "ws-write-id"})
		claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
		ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
		api.MetricsHandler(w, r.WithContext(ctx))
	}))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	ws, _, _ := websocket.DefaultDialer.Dial(u, nil)
	// Immediately close to force WriteMessage to fail
	ws.Close()
	// The handler runs in a goroutine, allow it to fail and exit
}

func TestLogsHandlerWS_WriteFail(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockWSClient{
		logsFunc: func(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
			return infiniteReader{}, nil // actually, we should send infinite header+payload
			// The handler reads 8 byte header, then payload.
			// Handlers_test.go logs wrapper loops:
			// reader.Read(header) expects 8 bytes.
			// infiniteReader returns 16 bytes.
		},
	}
	// For Write fail, we just need ANY valid read that then gets written.
	// We can use the mockClient's default logsFunc which sends 1 byte payload and 8 byte header.
	// But it closes immediately. To make write fail, it needs to try writing when conn is closed.
	// Let's use a blocking reader that returns one message so we can close in the meantime.
	mockCli.logsFunc = func(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
		r, w := io.Pipe()
		go func() {
			_, _ = w.Write(append([]byte{1, 0, 0, 0, 0, 0, 0, 5}, []byte("hello")...))
		}()
		return r, nil
	}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "ws-write-id2"}
	db.Create(&server)
	
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"id": "ws-write-id2"})
		claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
		ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
		api.LogsHandler(w, r.WithContext(ctx))
	}))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	ws, _, _ := websocket.DefaultDialer.Dial(u, nil)
	ws.Close() // Force write fail
}

func TestLogsHandlerWS_ReadFail(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockWSClient{
		logsFunc: func(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
			return failingReader{}, nil
		},
	}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "ws-read-id"}
	db.Create(&server)
	
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"id": "ws-read-id"})
		claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
		ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
		api.LogsHandler(w, r.WithContext(ctx))
	}))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	ws, _, _ := websocket.DefaultDialer.Dial(u, nil)
	ws.Close()

	// Give the server time to process the close and hit the error return branches
	time.Sleep(20 * time.Millisecond)
}
