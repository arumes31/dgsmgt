package api

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/gorilla/mux"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type mockClient struct {
	inspectFunc func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	listFunc    func(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	startErr    error
	stopErr     error
	restartErr  error
	logsErr     error
	createErr   error
	removeErr   error
	pullErr     error
	statsErr    error
}

func (m *mockClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, containerID)
	}
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    "1234567890abcdef",
			State: &types.ContainerState{Status: "running"},
		},
		Config: &container.Config{Image: "img"},
	}, nil
}

func (m *mockClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return m.startErr
}
func (m *mockClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return m.stopErr
}
func (m *mockClient) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	return m.restartErr
}
func (m *mockClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.logsErr != nil { return nil, m.logsErr }
	return io.NopCloser(strings.NewReader("log line")), nil
}
func (m *mockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	if m.createErr != nil { return container.CreateResponse{}, m.createErr }
	return container.CreateResponse{ID: "new-id"}, nil
}
func (m *mockClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	return m.removeErr
}
func (m *mockClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, options)
	}
	if m.pullErr != nil { return nil, m.pullErr } // Reusing pullErr for general list fail if needed or just return empty
	return []types.Container{}, nil
}
func (m *mockClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	if m.pullErr != nil { return nil, m.pullErr }
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	if m.statsErr != nil { return container.StatsResponseReader{}, m.statsErr }
	return container.StatsResponseReader{}, nil
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	_ = db.AutoMigrate(&models.User{}, &models.Server{}, &models.UserServer{}, &models.AuditLog{})
	return db
}

// Helper to get a DB that will fail for a specific table
func setupFailDB(t *testing.T, model interface{}) *gorm.DB {
	db := setupTestDB(t)
	_ = db.Migrator().DropTable(model)
	return db
}

func TestHealthHandler(t *testing.T) {
	api := NewAPI(nil, nil, "secret", nil, zap.NewNop())
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	
	api.HealthHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestLoginHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	hashedPass, _ := auth.HashPassword("password123")
	user := models.User{Username: "testuser", PasswordHash: hashedPass}
	db.Create(&user)
	
	payload := `{"username":"testuser","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(payload))
	w := httptest.NewRecorder()
	
	api.LoginHandler(w, req, "secret")
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Token == "" {
		t.Error("Expected token in response")
	}
}

func TestMeHandler(t *testing.T) {
	api := NewAPI(nil, nil, "secret", nil, zap.NewNop())
	claims := &auth.Claims{UserID: 1, Username: "testuser"}
	req := httptest.NewRequest("GET", "/api/me", nil)
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.MeHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestListMyServersHandler(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "cont-id"}
	db.Create(&server)
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("GET", "/api/my-servers", nil)
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.ListMyServersHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestStatusHandler(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "1234567890abcdef"}
	db.Create(&server)
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("GET", "/api/status/1234567890abcdef", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1234567890abcdef"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.StatusHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestActionHandler(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "test-server", ContainerID: "1234567890abcdef"}
	db.Create(&server)
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	
	actions := []string{"start", "stop", "restart"}
	for _, action := range actions {
		req := httptest.NewRequest("POST", "/api/action/1234567890abcdef/"+action, nil)
		req = mux.SetURLVars(req, map[string]string{"id": "1234567890abcdef", "action": action})
		ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		
		api.ActionHandler(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Action %s: Expected status 200, got %d", action, w.Code)
		}
	}
}

func TestCreateUserHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	payload := `{"username":"newuser","password":"password123","is_admin":false}`
	req := httptest.NewRequest("POST", "/api/admin/users", strings.NewReader(payload))
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.CreateUserHandler(w, req)
	
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}
}

func TestDeleteUserHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	user := models.User{Username: "todelete"}
	db.Create(&user)
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("DELETE", "/api/admin/users/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.DeleteUserHandler(w, req)
	
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

func TestCreateServerHandler(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	payload := `{"name":"new-server","image":"alpine","ports":["8080:80"]}`
	req := httptest.NewRequest("POST", "/api/admin/servers", strings.NewReader(payload))
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.CreateServerHandler(w, req)
	
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}
}

func TestAssignServerHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	payload := `{"user_id":1,"server_id":1,"can_start":true}`
	req := httptest.NewRequest("POST", "/api/admin/assign", strings.NewReader(payload))
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.AssignServerHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestChangePasswordHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	user := models.User{Username: "testuser"}
	db.Create(&user)
	
	claims := &auth.Claims{UserID: user.ID, Username: "testuser"}
	payload := `{"password":"newpassword123"}`
	req := httptest.NewRequest("POST", "/api/me/password", strings.NewReader(payload))
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.ChangePasswordHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestListUsersHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	db.Create(&models.User{Username: "user1"})
	
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	w := httptest.NewRecorder()
	
	api.ListUsersHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestUpdateUserHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	user := models.User{Username: "oldname"}
	db.Create(&user)
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	payload := `{"username":"newname","is_admin":true}`
	req := httptest.NewRequest("PUT", "/api/admin/users/1", strings.NewReader(payload))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.UpdateUserHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestDeleteServerHandler(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewNop())
	
	server := models.Server{Name: "todelete", ContainerID: "cont-id"}
	db.Create(&server)
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("DELETE", "/api/admin/servers/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.DeleteServerHandler(w, req)
	
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

func TestListAssignmentsHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	db.Create(&models.UserServer{UserID: 1, ServerID: 1})
	
	req := httptest.NewRequest("GET", "/api/admin/assignments", nil)
	w := httptest.NewRecorder()
	
	api.ListAssignmentsHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestDeleteAssignmentHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	db.Create(&models.UserServer{UserID: 1, ServerID: 1})
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("DELETE", "/api/admin/assignments/1/1", nil)
	req = mux.SetURLVars(req, map[string]string{"userId": "1", "serverId": "1"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.DeleteAssignmentHandler(w, req)
	
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

func TestListAuditLogsHandler(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	db.Create(&models.AuditLog{Username: "testuser", Action: "test"})
	
	req := httptest.NewRequest("GET", "/api/admin/audit-logs", nil)
	w := httptest.NewRecorder()
	
	api.ListAuditLogsHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestListServersHandlerAdmin(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	db.Create(&models.Server{Name: "server1"})
	
	req := httptest.NewRequest("GET", "/api/admin/servers", nil)
	w := httptest.NewRecorder()
	
	api.ListServersHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestActionHandlerNotFound(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("POST", "/api/action/nonexistent/start", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "nonexistent", "action": "start"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.ActionHandler(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	api := NewAPI(nil, db, "secret", nil, zap.NewNop())
	
	claims := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}
	req := httptest.NewRequest("DELETE", "/api/admin/users/999", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "999"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	api.DeleteUserHandler(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestUnauthorizedAction(t *testing.T) {
	db := setupTestDB(t)
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	api := NewAPI(svc, db, "secret", nil, zap.NewExample())
	
	server := models.Server{Name: "test", ContainerID: "cont-id-123"}
	db.Create(&server)
	if server.ID == 0 {
		t.Fatal("Server ID not set after create")
	}
	
	// User with assignment but no start permission
	us := models.UserServer{
		UserID: 2, 
		ServerID: server.ID, 
	}
	db.Create(&us)
	db.Model(&us).Updates(map[string]interface{}{
		"can_start":     false,
		"can_stop":      true,
		"can_restart":   true,
		"can_view_logs": true,
	})
	
	claims := &auth.Claims{UserID: 2, Username: "regular", IsAdmin: false}
	req := httptest.NewRequest("POST", "/api/action/cont-id-123/start", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "cont-id-123", "action": "start"})
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	
	// Verify canAccess separately first
	if !api.canAccess(claims, "cont-id-123") {
		t.Fatal("canAccess returned false, expected true")
	}
	
	api.ActionHandler(w, req)
	
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d. Body: %s", w.Code, w.Body.String())
	}
}
