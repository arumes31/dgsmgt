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

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func TestHandlers_GORM_Failures(t *testing.T) {
	adminClaims := &auth.Claims{UserID: 1, IsAdmin: true}
	userClaims := &auth.Claims{UserID: 2, IsAdmin: false}

	t.Run("ChangePassword_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("POST", "/api/me/password", strings.NewReader(`{"password":"newpassword123"}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, userClaims))
		w := httptest.NewRecorder()
		api.ChangePasswordHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Login_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"testuser","password":"password123"}`))
		w := httptest.NewRecorder()
		api.LoginHandler(w, req, "secret")
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Action_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.Server{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("POST", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "abc", "action": "start"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.ActionHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("ListMyServers_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.Server{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.ListMyServersHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_ListUsers_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		api.ListUsersHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_CreateUser_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"username":"newuser","password":"password123"}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_UpdateUser_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"username":"updateduser","password":"password123"}`))
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_DeleteUser_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_ListServers_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.Server{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		api.ListServersHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_DeleteServer_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.Server{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteServerHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_Assign_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.UserServer{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"user_id":1,"server_id":1}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.AssignServerHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_DeleteAssignment_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.UserServer{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"userId": "1", "serverId": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteAssignmentHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_AuditLogs_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.AuditLog{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		api.ListAuditLogsHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Admin_ListAssignments_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.UserServer{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		api.ListAssignmentsHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})
}

func TestHandlers_ValidationFailures(t *testing.T) {
	adminClaims := &auth.Claims{UserID: 1, IsAdmin: true}
	api := NewAPI(nil, nil, "secret", nil, zap.NewNop())

	t.Run("CreateUser_InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{invalid`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateUserHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})

	t.Run("CreateUser_ValidationFail", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"username":"u"}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateUserHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})

	t.Run("UpdateUser_InvalidJSON", func(t *testing.T) {
		db := setupTestDB(t)
		db.Create(&models.User{Username: "test"})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("PUT", "/", strings.NewReader(`{invalid`))
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})

	t.Run("UpdateUser_ValidationFail", func(t *testing.T) {
		db := setupTestDB(t)
		db.Create(&models.User{Username: "test"})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"username":"u"}`))
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})
}

func TestHandlers_Docker_Failures(t *testing.T) {
	adminClaims := &auth.Claims{UserID: 1, IsAdmin: true}
	
	t.Run("CreateServer_ImagePullFail", func(t *testing.T) {
		db := setupTestDB(t)
		mc := &mockClient{pullErr: fmt.Errorf("pull error")}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, db, "secret", nil, zap.NewNop())
		
		payload := `{"name":"n","image":"i","ports":[]}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(payload))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateServerHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("CreateServer_ContainerCreateFail", func(t *testing.T) {
		db := setupTestDB(t)
		mc := &mockClient{createErr: fmt.Errorf("create error")}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, db, "secret", nil, zap.NewNop())
		
		payload := `{"name":"n","image":"i","ports":[]}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(payload))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateServerHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("DeleteServer_ContainerRemoveFail", func(t *testing.T) {
		db := setupTestDB(t)
		mc := &mockClient{removeErr: fmt.Errorf("remove error")}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, db, "secret", nil, zap.NewNop())
		
		db.Create(&models.Server{Name: "n", ContainerID: "c"})
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteServerHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("Action_StartFail", func(t *testing.T) {
		db := setupTestDB(t)
		mc := &mockClient{startErr: fmt.Errorf("start error")}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, db, "secret", nil, zap.NewNop())
		
		db.Create(&models.Server{Name: "n", ContainerID: "c"})
		req := httptest.NewRequest("POST", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "c", "action": "start"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.ActionHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("UpdateUser_NotFound", func(t *testing.T) {
		db := setupTestDB(t)
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"username":"u","is_admin":true}`))
		req = mux.SetURLVars(req, map[string]string{"id": "999"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusNotFound { t.Errorf("expected 404, got %d", w.Code) }
	})

	t.Run("MetricsHandler_StatsFail", func(t *testing.T) {
		mc := &mockClient{statsErr: fmt.Errorf("stats error")}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, nil, "secret", nil, zap.NewNop())
		
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = mux.SetURLVars(r, map[string]string{"id": "1"})
			r = r.WithContext(context.WithValue(r.Context(), middleware.ClaimsKey, adminClaims))
			api.MetricsHandler(w, r)
		}))
		defer server.Close()

		url := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil { t.Fatalf("dial failed: %v", err) }
		defer conn.Close()

		_, p, err := conn.ReadMessage()
		if err != nil { t.Fatalf("read failed: %v", err) }
		if !strings.Contains(string(p), "Error reading stats") {
			t.Errorf("expected error message, got %s", string(p))
		}
	})

	t.Run("LogsHandler_LogsFail", func(t *testing.T) {
		mc := &mockClient{logsErr: fmt.Errorf("logs error")}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, nil, "secret", nil, zap.NewNop())
		
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = mux.SetURLVars(r, map[string]string{"id": "1"})
			r = r.WithContext(context.WithValue(r.Context(), middleware.ClaimsKey, adminClaims))
			api.LogsHandler(w, r)
		}))
		defer server.Close()

		url := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil { t.Fatalf("dial failed: %v", err) }
		defer conn.Close()

		_, p, err := conn.ReadMessage()
		if err != nil { t.Fatalf("read failed: %v", err) }
		if !strings.Contains(string(p), "Error reading logs") {
			t.Errorf("expected error message, got %s", string(p))
		}
	})
}

func TestHandlers_DeepCoverage(t *testing.T) {
	adminClaims := &auth.Claims{UserID: 1, IsAdmin: true}

	t.Run("UpdateUser_PasswordBranch", func(t *testing.T) {
		db := setupTestDB(t)
		user := models.User{Username: "test"}
		db.Create(&user)
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		
		// Test with password
		req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"username":"test","password":"newpassword123"}`))
		req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", user.ID)})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusOK { t.Errorf("expected 200, got %d", w.Code) }

		// Test without password
		req = httptest.NewRequest("PUT", "/", strings.NewReader(`{"username":"test2"}`))
		req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", user.ID)})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w = httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusOK { t.Errorf("expected 200, got %d", w.Code) }
	})
}
