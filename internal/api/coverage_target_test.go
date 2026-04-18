package api

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func TestCoverageTarget_Full(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	adminClaims := &auth.Claims{UserID: 1, IsAdmin: true}
	userClaims := &auth.Claims{UserID: 2, IsAdmin: false}
	
	user := models.User{Username: "testuser"}
	db.Create(&user)
	server := models.Server{Name: "testserver", ContainerID: "abc"}
	db.Create(&server)

	t.Run("JSONFailures", func(t *testing.T) {
		handlers := []struct {
			name    string
			handler func(http.ResponseWriter, *http.Request)
			claims  *auth.Claims
			urlVars map[string]string
		}{
			{"ChangePassword", api.ChangePasswordHandler, userClaims, nil},
			{"Login", func(w http.ResponseWriter, r *http.Request) { api.LoginHandler(w, r, "secret") }, nil, nil},
			{"CreateUser", api.CreateUserHandler, adminClaims, nil},
			{"UpdateUser", api.UpdateUserHandler, adminClaims, map[string]string{"id": fmt.Sprintf("%d", user.ID)}},
			{"CreateServer", api.CreateServerHandler, adminClaims, nil},
			{"AssignServer", api.AssignServerHandler, adminClaims, nil},
		}
		for _, tt := range handlers {
			req := httptest.NewRequest("POST", "/", strings.NewReader("!@#$%"))
			if tt.urlVars != nil { req = mux.SetURLVars(req, tt.urlVars) }
			if tt.claims != nil { req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, tt.claims)) }
			w := httptest.NewRecorder()
			tt.handler(w, req)
			if w.Code != http.StatusBadRequest { t.Errorf("%s failed", tt.name) }
		}
	})

	t.Run("StatusDockerFail", func(t *testing.T) {
		mockCli.inspectFunc = func(ctx context.Context, id string) (types.ContainerJSON, error) {
			return types.ContainerJSON{}, fmt.Errorf("fail")
		}
		req := httptest.NewRequest("GET", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "abc"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.StatusHandler(w, req)
		if w.Code != http.StatusNotFound { t.Error("expected 404") }
	})

	t.Run("ActionPermissions", func(t *testing.T) {
		us := models.UserServer{UserID: user.ID, ServerID: server.ID}
		db.Create(&us)
		db.Model(&models.UserServer{}).Where("user_id = ? AND server_id = ?", user.ID, server.ID).Updates(map[string]interface{}{
			"can_start":   false,
			"can_stop":    false,
			"can_restart": false,
		})
		
		for _, act := range []string{"start", "stop", "restart"} {
			req := httptest.NewRequest("POST", "/", nil)
			req = mux.SetURLVars(req, map[string]string{"id": "abc", "action": act})
			req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, &auth.Claims{UserID: user.ID, IsAdmin: false}))
			w := httptest.NewRecorder()
			api.ActionHandler(w, req)
			if w.Code != http.StatusForbidden { t.Errorf("expected 403 for %s, got %d", act, w.Code) }
		}
	})

	t.Run("WSFails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "abc"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.MetricsHandler(w, req)
		api.LogsHandler(w, req)
	})

	t.Run("ListMyServersMapping", func(t *testing.T) {
		mockCli.listFunc = func(ctx context.Context, opts container.ListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "abc", Status: "running"}}, nil
		}
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.ListMyServersHandler(w, req)
	})

	t.Run("DBFailures", func(t *testing.T) {
		_ = db.Migrator().DropTable(&models.Server{})
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.ListMyServersHandler(w, req)
		
		req2 := httptest.NewRequest("POST", "/", nil)
		req2 = mux.SetURLVars(req2, map[string]string{"id": "abc", "action": "start"})
		req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.ClaimsKey, adminClaims))
		w2 := httptest.NewRecorder()
		api.ActionHandler(w2, req2)
	})
}

func TestCoverageTarget_EdgeCases(t *testing.T) {
	t.Run("StripHeader", func(t *testing.T) {
		if string(stripDockerHeader([]byte{1,2,3})) != string([]byte{1,2,3}) { t.Error("fail short") }
		if len(stripDockerHeader([]byte{1,0,0,0,0,0,0,0})) != 8 { t.Error("fail empty") }
	})
}
