package api

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type gapErrorReader struct {
	data []byte
	err  error
	sent bool
}

func (e *gapErrorReader) Read(p []byte) (n int, err error) {
	if !e.sent {
		n = copy(p, e.data)
		e.sent = true
		return n, nil
	}
	return 0, e.err
}

func (e *gapErrorReader) Close() error { return nil }

func TestLogsHandler_ReaderError(t *testing.T) {
	db := setupTestDB(t)
	
	// Create a reader that sends a valid docker header + payload, then fails with a non-EOF error
	header := []byte{1, 0, 0, 0, 0, 0, 0, 5}
	payload := []byte("hello")
	fullData := append(header, payload...)
	
	mockCli := &mockWSClient{
		logsFunc: func(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
			return &gapErrorReader{data: fullData, err: errors.New("simulated error")}, nil
		},
	}
	
	svc := docker.NewServiceWithClient(mockCli)
	logger := zap.NewNop() // We just need to hit the line, even with Nop
	api := NewAPI(svc, db, "secret", nil, logger)
	
	server := models.Server{Name: "test-server", ContainerID: "gap-id"}
	db.Create(&server)
	
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"id": "gap-id"})
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

	// Read the successful "hello" message
	_, message, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if string(message) != "hello" {
		t.Errorf("expected hello, got %s", string(message))
	}

	// Give it a moment to hit the error branch in the loop
	time.Sleep(50 * time.Millisecond)
}
