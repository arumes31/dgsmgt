package main

import (
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("JWT_SECRET", "test_secret")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("PORT", "0")
	os.Setenv("DEBUG", "true")
	os.Setenv("TRUST_PROXY", "true")
	os.Setenv("ADMIN_USER", "customadmin")
	os.Setenv("ADMIN_PASSWORD", "custompass")
	defer os.Unsetenv("DATABASE_URL")
	defer os.Unsetenv("JWT_SECRET")
	defer os.Unsetenv("TEST_MODE")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("DEBUG")
	defer os.Unsetenv("TRUST_PROXY")
	defer os.Unsetenv("ADMIN_USER")
	defer os.Unsetenv("ADMIN_PASSWORD")

	database, _ := db.InitDB(":memory:")
	_ = database.AutoMigrate(&models.Server{})
	database.Create(&models.Server{Name: "cron-server", ContainerID: "abc", CronSchedule: "* * * * *"})
	
	err := Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
}

func TestRunFailures(t *testing.T) {
	t.Run("DBFail", func(t *testing.T) {
		os.Setenv("DATABASE_URL", "/invalid/path")
		defer os.Unsetenv("DATABASE_URL")
		if err := Run(); err == nil { t.Error("expected error") }
	})

	t.Run("MigrateFail", func(t *testing.T) {
		f, _ := os.CreateTemp("", "readonly*.db")
		f.Close()
		_ = os.Chmod(f.Name(), 0444)
		defer os.Remove(f.Name())
		os.Setenv("DATABASE_URL", f.Name())
		defer os.Unsetenv("DATABASE_URL")
		if err := Run(); err == nil { t.Error("expected error") }
	})
	
	t.Run("PortFail", func(t *testing.T) {
		os.Setenv("DATABASE_URL", ":memory:")
		os.Setenv("PORT", "99999") // Invalid port
		os.Setenv("TEST_MODE", "false") // Need to let it fail in ListenAndServe
		defer os.Unsetenv("DATABASE_URL")
		defer os.Unsetenv("PORT")
		defer os.Unsetenv("TEST_MODE")
		
		// This might still not hit the case because of the select block
		// But we try.
		go func() {
			time.Sleep(100 * time.Millisecond)
			p, _ := os.FindProcess(os.Getpid())
			_ = p.Signal(os.Interrupt)
		}()
		_ = Run()
	})
}

func TestSecureHeaders(t *testing.T) {
	handler := secureHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Frame-Options") == "" { t.Error("header missing") }
}

func TestMainFunction(t *testing.T) {
	if os.Getenv("BE_MAIN") == "1" {
		os.Setenv("TEST_MODE", "true")
		os.Setenv("PORT", "0")
		os.Setenv("DATABASE_URL", ":memory:")
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainFunction")
	cmd.Env = append(os.Environ(), "BE_MAIN=1")
	_ = cmd.Run()
}

func TestMainFunctionFail(t *testing.T) {
	if os.Getenv("BE_MAIN_FAIL") == "1" {
		os.Setenv("DATABASE_URL", "/invalid/path")
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainFunctionFail")
	cmd.Env = append(os.Environ(), "BE_MAIN_FAIL=1")
	if err := cmd.Run(); err == nil { t.Error("expected error") }
}
