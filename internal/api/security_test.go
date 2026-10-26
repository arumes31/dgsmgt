package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"

	"go.uber.org/zap"
)

func newTestAPI(t *testing.T) *API {
	t.Helper()
	database, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := database.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := database.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	return NewAPI(docker.NewServiceWithClient(&mockClient{}), database, "0123456789abcdef0123456789abcdef", nil, zap.NewNop())
}

func TestAuthorizeUsesStoredContainerID(t *testing.T) {
	a := newTestAPI(t)
	user := models.User{Username: "user1", PasswordHash: "x"}
	a.db.Create(&user)
	own := models.Server{Name: "own", ContainerID: "cafe1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"}
	other := models.Server{Name: "other", ContainerID: "beef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"}
	a.db.Create(&own)
	a.db.Create(&other)
	a.db.Create(&models.UserServer{UserID: user.ID, ServerID: own.ID, CanViewLogs: true})

	claims := &auth.Claims{UserID: user.ID}

	tests := []struct {
		id     string
		ok     bool
		target string
	}{
		{own.ContainerID, true, own.ContainerID},
		{own.ContainerID[:12], true, own.ContainerID},
		// Short or wildcard IDs must not match by prefix
		{"cafe", false, ""},
		{"%", false, ""},
		{"c_fe%", false, ""},
		{"cafe1234567_%", false, ""},
		{other.ContainerID, false, ""},
	}
	for _, tt := range tests {
		_, _, target, ok := a.authorize(claims, tt.id)
		if ok != tt.ok || target != tt.target {
			t.Errorf("authorize(%q) = (%q, %v), want (%q, %v)", tt.id, target, ok, tt.target, tt.ok)
		}
	}
}

func TestAuthMiddlewareRevocation(t *testing.T) {
	a := newTestAPI(t)
	user := models.User{Username: "admin1", PasswordHash: "x", IsAdmin: true}
	a.db.Create(&user)
	token, err := auth.GenerateToken(&user, a.jwtSecret)
	if err != nil {
		t.Fatal(err)
	}

	handler := middleware.AuthMiddleware(a.jwtSecret, a.db, a.tickets)(middleware.AdminMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	call := func() int {
		req := httptest.NewRequest("GET", "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	if code := call(); code != http.StatusOK {
		t.Fatalf("valid token: got %d", code)
	}

	// Demotion takes effect immediately even though the token still says is_admin
	a.db.Model(&user).Update("is_admin", false)
	if code := call(); code != http.StatusForbidden {
		t.Fatalf("demoted admin: got %d, want 403", code)
	}

	// Bumping the token version revokes the token
	a.db.Model(&user).Update("token_version", 1)
	if code := call(); code != http.StatusUnauthorized {
		t.Fatalf("revoked token: got %d, want 401", code)
	}

	// Cookies are no longer accepted
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("cookie auth: got %d, want 401", w.Code)
	}
}

func TestCheckOrigin(t *testing.T) {
	a := newTestAPI(t)
	a.allowedOrigins = []string{"https://panel.example.com"}
	for origin, want := range map[string]bool{
		"":                          true,
		"http://example.org":        true, // same host as the request
		"https://panel.example.com": true,
		"https://evil.example":      false,
	} {
		req := httptest.NewRequest("GET", "http://example.org/api/logs/x", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if got := a.checkOrigin(req); got != want {
			t.Errorf("checkOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}

func TestLoginLimiter(t *testing.T) {
	l := newLoginLimiter()
	for i := 0; i < maxFailuresPerIPUser; i++ {
		if l.Blocked("1.2.3.4", "admin") {
			t.Fatalf("blocked too early after %d failures", i)
		}
		l.Failure("1.2.3.4", "admin")
	}
	if !l.Blocked("1.2.3.4", "admin") {
		t.Fatal("expected lockout after repeated failures")
	}
	if l.Blocked("5.6.7.8", "admin") {
		t.Fatal("other IPs must not be locked out by a single IP")
	}

	// Many usernames from one IP hit the per-IP limit
	l2 := newLoginLimiter()
	for i := 0; i < maxFailuresPerIP; i++ {
		l2.Failure("1.2.3.4", string(rune('a'+i%26))+"user")
	}
	if !l2.Blocked("1.2.3.4", "fresh") {
		t.Fatal("expected per-IP lockout")
	}
}
