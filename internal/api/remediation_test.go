package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dgsmgt/internal/auth"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"

	"github.com/gorilla/mux"
)

func withClaims(r *http.Request, c *auth.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), middleware.ClaimsKey, c))
}

func TestAssignServerPreservesDeniedPermissions(t *testing.T) {
	a := newTestAPI(t)
	admin := &auth.Claims{UserID: 1, Username: "admin", IsAdmin: true}

	assign := func(body map[string]any) {
		t.Helper()
		data, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		a.AssignServerHandler(w, withClaims(httptest.NewRequest("POST", "/api/admin/assign", bytes.NewReader(data)), admin))
		if w.Code != http.StatusOK {
			t.Fatalf("assign: got %d: %s", w.Code, w.Body.String())
		}
	}
	load := func() models.UserServer {
		t.Helper()
		var us models.UserServer
		if err := a.db.Where("user_id = ? AND server_id = ?", 7, 9).First(&us).Error; err != nil {
			t.Fatal(err)
		}
		return us
	}

	// Creation with every permission denied
	assign(map[string]any{"user_id": 7, "server_id": 9, "can_start": false, "can_stop": false, "can_restart": false, "can_view_logs": false})
	if us := load(); us.CanStart || us.CanStop || us.CanRestart || us.CanViewLogs {
		t.Fatalf("created assignment broadened permissions: %+v", us)
	}
	// Update to grant some, then deny again
	assign(map[string]any{"user_id": 7, "server_id": 9, "can_start": true, "can_view_logs": true})
	if us := load(); !us.CanStart || us.CanStop || us.CanRestart || !us.CanViewLogs {
		t.Fatalf("updated assignment mismatch: %+v", us)
	}
	assign(map[string]any{"user_id": 7, "server_id": 9, "can_start": false, "can_view_logs": false})
	if us := load(); us.CanStart || us.CanViewLogs {
		t.Fatalf("denied permissions were not persisted: %+v", us)
	}
}

func TestChangePasswordRejectsStaleVersion(t *testing.T) {
	a := newTestAPI(t)
	user := models.User{Username: "user1", PasswordHash: "x", TokenVersion: 3}
	a.db.Create(&user)

	change := func(ver uint) *httptest.ResponseRecorder {
		body := bytes.NewBufferString(`{"password":"a-strong-new-password"}`)
		w := httptest.NewRecorder()
		a.ChangePasswordHandler(w, withClaims(httptest.NewRequest("POST", "/api/me/password", body), &auth.Claims{UserID: user.ID, TokenVersion: ver}))
		return w
	}

	if w := change(3); w.Code != http.StatusOK {
		t.Fatalf("change: got %d: %s", w.Code, w.Body.String())
	}
	var stored models.User
	a.db.First(&stored, user.ID)
	if stored.TokenVersion != 4 {
		t.Fatalf("token version = %d, want 4", stored.TokenVersion)
	}
	// A second request carrying the now-revoked version must not apply
	if w := change(3); w.Code != http.StatusConflict {
		t.Fatalf("stale change: got %d, want 409", w.Code)
	}
	a.db.First(&stored, user.ID)
	if stored.TokenVersion != 4 {
		t.Fatalf("stale change modified token version to %d", stored.TokenVersion)
	}
}

func TestUpdateUserRevokesWithDistinctVersion(t *testing.T) {
	a := newTestAPI(t)
	user := models.User{Username: "user1", PasswordHash: "x", TokenVersion: 2}
	a.db.Create(&user)
	admin := &auth.Claims{UserID: 99, Username: "admin", IsAdmin: true}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/admin/users/1", bytes.NewBufferString(`{"username":"user1","password":"another-strong-pass"}`))
	req = mux.SetURLVars(withClaims(req, admin), map[string]string{"id": "1"})
	a.UpdateUserHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d: %s", w.Code, w.Body.String())
	}
	var stored models.User
	a.db.First(&stored, user.ID)
	if stored.TokenVersion != 3 || stored.PasswordHash == "x" {
		t.Fatalf("reset not applied: version %d", stored.TokenVersion)
	}
}

func TestStreamAccessRevalidation(t *testing.T) {
	a := newTestAPI(t)
	user := models.User{Username: "user1", PasswordHash: "x"}
	a.db.Create(&user)
	server := models.Server{Name: "srv", ContainerID: "cafe1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"}
	a.db.Create(&server)
	a.db.Create(&models.UserServer{UserID: user.ID, ServerID: server.ID, CanViewLogs: true})
	claims := &auth.Claims{UserID: user.ID, TokenVersion: 0}

	if !a.streamAccessAllowed(claims, server.ContainerID, true) {
		t.Fatal("assigned user must keep access")
	}
	a.db.Model(&models.UserServer{}).Where("user_id = ?", user.ID).Update("can_view_logs", false)
	if a.streamAccessAllowed(claims, server.ContainerID, true) {
		t.Fatal("revoked log permission must stop the log stream")
	}
	if !a.streamAccessAllowed(claims, server.ContainerID, false) {
		t.Fatal("metrics only need the assignment")
	}
	a.db.Where("user_id = ?", user.ID).Delete(&models.UserServer{})
	if a.streamAccessAllowed(claims, server.ContainerID, false) {
		t.Fatal("removed assignment must stop the stream")
	}
	a.db.Create(&models.UserServer{UserID: user.ID, ServerID: server.ID, CanViewLogs: true})
	a.db.Model(&models.User{}).Where("id = ?", user.ID).Update("token_version", 1)
	if a.streamAccessAllowed(claims, server.ContainerID, true) {
		t.Fatal("revoked session must stop the stream")
	}
	a.db.Model(&models.User{}).Where("id = ?", user.ID).Update("token_version", 0)
	a.db.Delete(&user)
	if a.streamAccessAllowed(claims, server.ContainerID, true) {
		t.Fatal("deleted account must stop the stream")
	}
}
