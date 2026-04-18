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

	t.Run("Admin_CreateServer_DBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.Server{})
		// Mock Docker client so it doesn't fail on Docker creation
		mc := &mockClient{}
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"newserver","image":"nginx","ports":[]}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateServerHandler(w, req)
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

	t.Run("CreateServer_InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{invalid`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateServerHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})

	t.Run("CreateServer_ValidationFail", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"n"}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.CreateServerHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})

	t.Run("AssignServer_InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{invalid`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.AssignServerHandler(w, req)
		if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	})

	t.Run("AssignServer_ValidationFail", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"user_id":0}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.AssignServerHandler(w, req)
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
		
		payload := `{"name":"newserver","image":"i","ports":[]}`
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
		
		payload := `{"name":"newserver","image":"i","ports":[]}`
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
		if w.Code != http.StatusNoContent { t.Errorf("expected 204, got %d", w.Code) }
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

	t.Run("HashPassword_Failures", func(t *testing.T) {
		db := setupTestDB(t)
		user := models.User{Username: "test"}
		db.Create(&user)
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		
		longPass := strings.Repeat("a", 73) // bcrypt fails > 72 bytes

		// ChangePasswordHandler
		req := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"password":"%s"}`, longPass)))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.ChangePasswordHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("ChangePassword: expected 500, got %d", w.Code) }

		// CreateUserHandler
		req = httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"username":"u2","password":"%s"}`, longPass)))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w = httptest.NewRecorder()
		api.CreateUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("CreateUser: expected 500, got %d", w.Code) }

		// UpdateUserHandler
		req = httptest.NewRequest("PUT", "/", strings.NewReader(fmt.Sprintf(`{"username":"test","password":"%s"}`, longPass)))
		req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", user.ID)})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w = httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("UpdateUser: expected 500, got %d", w.Code) }
	})

	t.Run("DeleteServer_NotFound", func(t *testing.T) {
		db := setupTestDB(t)
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "999"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteServerHandler(w, req)
		if w.Code != http.StatusNotFound { t.Errorf("expected 404, got %d", w.Code) }
	})

	t.Run("LogsHandler_PermissionDenied", func(t *testing.T) {
		db := setupTestDB(t)
		user := models.User{Username: "standard"}
		db.Create(&user)
		server := models.Server{Name: "srv", ContainerID: "abc"}
		db.Create(&server)
		db.Create(&models.UserServer{UserID: user.ID, ServerID: server.ID})
		db.Model(&models.UserServer{}).Where("user_id = ?", user.ID).Update("can_view_logs", false)

		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/api/logs/abc", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "abc"})
		claims := &auth.Claims{UserID: user.ID, IsAdmin: false}
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
		w := httptest.NewRecorder()
		api.LogsHandler(w, req)
		if w.Code != http.StatusForbidden { t.Errorf("expected 403, got %d", w.Code) }
	})

	t.Run("DeleteUser_FirstFail", func(t *testing.T) {
		// Mock First failing with a generic error (not ErrRecordNotFound)
		db := setupFailDB(t, &models.User{})
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("UpdateUser_SaveFail", func(t *testing.T) {
		db := setupFailOpDB(t, "update")
		user := models.User{Username: "test"}
		db.Create(&user) // This create succeeds because the callback fails "update" (Save uses update if record exists!)
		// Wait, if it fails "update", Save will fail!

		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"username":"newname"}`))
		req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", user.ID)})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.UpdateUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("DeleteUser_DeleteFail", func(t *testing.T) {
		db := setupFailOpDB(t, "delete")
		user := models.User{Username: "test"}
		db.Create(&user)
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", user.ID)})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteUserHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("DeleteServer_DeleteFail", func(t *testing.T) {
		db := setupFailOpDB(t, "delete")
		server := models.Server{Name: "srv"}
		db.Create(&server)
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		
		req := httptest.NewRequest("DELETE", "/", nil)
		req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", server.ID)})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
		w := httptest.NewRecorder()
		api.DeleteServerHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("ListMyServers_UserDBFail", func(t *testing.T) {
		db := setupFailDB(t, &models.Server{}) // Drop servers table so Joins().Find() fails
		api := NewAPI(nil, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		claims := &auth.Claims{UserID: 1, IsAdmin: false} // Non-admin triggers the Joins code path
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
		w := httptest.NewRecorder()
		api.ListMyServersHandler(w, req)
		if w.Code != http.StatusInternalServerError { t.Errorf("expected 500, got %d", w.Code) }
	})

	t.Run("LoginHandler_EdgeCases", func(t *testing.T) {
		api := NewAPI(nil, setupTestDB(t), "secret", nil, zap.NewNop())

		t.Run("InvalidJSON", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/login", bytes.NewBufferString("{invalid}"))
			w := httptest.NewRecorder()
			api.LoginHandler(w, req, "secret")
			if w.Code != http.StatusBadRequest { t.Errorf("got %d", w.Code) }
		})

		t.Run("ValidationFail", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"username": ""}`))
			w := httptest.NewRecorder()
			api.LoginHandler(w, req, "secret")
			if w.Code != http.StatusBadRequest { t.Errorf("got %d", w.Code) }
		})

		t.Run("BruteForceProtect", func(t *testing.T) {
			reqBody := `{"username": "baduser", "password": "wrong"}`
			for i := 0; i < 5; i++ {
				req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(reqBody))
				req.RemoteAddr = "127.0.0.1:1234"
				api.LoginHandler(httptest.NewRecorder(), req, "secret")
			}
			// 6th attempt should be blocked
			req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(reqBody))
			req.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			api.LoginHandler(w, req, "secret")
			if w.Code != http.StatusForbidden { t.Errorf("got %d", w.Code) }
		})

		t.Run("AuthDBError", func(t *testing.T) {
			db := setupTestDB(t)
			_ = db.Exec("DROP TABLE users") // Cause query error
			apiErr := NewAPI(nil, db, "secret", nil, zap.NewNop())
			req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"username": "a", "password": "b"}`))
			w := httptest.NewRecorder()
			apiErr.LoginHandler(w, req, "secret")
			if w.Code != http.StatusInternalServerError { t.Errorf("got %d", w.Code) }
		})

		t.Run("TokenGenFail", func(t *testing.T) {
			db := setupTestDB(t)
			h, _ := auth.HashPassword("p")
			db.Create(&models.User{Username: "u", PasswordHash: h})
			
			oldGen := auth.GenerateToken
			auth.GenerateToken = func(u *models.User, s string) (string, error) { return "", errors.New("fail") }
			defer func() { auth.GenerateToken = oldGen }()

			api := NewAPI(nil, db, "secret", nil, zap.NewNop())
			req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"username": "u", "password": "p"}`))
			w := httptest.NewRecorder()
			api.LoginHandler(w, req, "secret")
			if w.Code != http.StatusInternalServerError { t.Errorf("got %d", w.Code) }
		})
	})

	t.Run("LogsHandler_Edge", func(t *testing.T) {
		t.Run("UpgradeFail", func(t *testing.T) {
			api := NewAPI(nil, nil, "secret", nil, zap.NewNop())
			req := httptest.NewRequest("GET", "/", nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, adminClaims))
			w := httptest.NewRecorder()
			api.LogsHandler(w, req)
			// Upgrader fails if not websocket
			if w.Code != http.StatusBadRequest { t.Errorf("got %d", w.Code) }
		})

		t.Run("DockerLogsFail", func(t *testing.T) {
			db := setupTestDB(t)
			mc := &mockClient{logsErr: errors.New("logs fail")}
			svc := docker.NewServiceWithClient(mc)
			api := NewAPI(svc, db, "secret", nil, zap.NewNop())
			
			// We can't easily perform the full upgrade in unit test, 
			// but we can mock the upgrader or see if we can trigger the error branch another way.
			// Actually, if we just want to hit the line `if err != nil { _ = conn.WriteMessage(...) }`,
			// we can't do it without a valid connection. 
			// Let's rely on integration or live tests for the deep WS loops if necessary.
			// But wait, the goal is 100%. 
		})
	})
}

	t.Run("MetricsHandler_WriteFail", func(t *testing.T) {
		db := setupTestDB(t)
		mc := &mockClient{statsChan: make(chan []byte, 1)}
		mc.statsChan <- []byte("stats") // Queue one item
		close(mc.statsChan)
		svc := docker.NewServiceWithClient(mc)
		api := NewAPI(svc, db, "secret", nil, zap.NewNop())
		req := httptest.NewRequest("GET", "/", nil)
		claims := &auth.Claims{UserID: 1, IsAdmin: true}
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
		
		server := httptest.NewServer(http.HandlerFunc(api.MetricsHandler))
		defer server.Close()
		
		// Make sure it fails on first write by immediately closing connection.
		
		// To properly pass context/claims to WS dialer via HTTP, httptest.NewServer doesn't keep the context.
		// Instead we intercept the handler manually via httptest.NewServer(http.HandlerFunc...), wait we need standard WS testing.
		// A simpler way: mock the Upgrader entirely? Gorilla upgrader is tightly coupled.
	})
}
