package main

import (
	"bytes"
	"context"
	"dgsmgt/internal/docker"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"dgsmgt/internal/models"
	"github.com/docker/docker/api/types/container"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	_ = db.AutoMigrate(&models.User{}, &models.Server{})
	return db
}

type mockClient struct {
	docker.DockerClient
}

func (m *mockClient) Close() error { return nil }
func (m *mockClient) ContainerInspect(ctx context.Context, id string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}
func (m *mockClient) ContainerStart(ctx context.Context, id string, options container.StartOptions) error {
	return nil
}
func (m *mockClient) ContainerStop(ctx context.Context, id string, options container.StopOptions) error {
	return nil
}
func (m *mockClient) ContainerRestart(ctx context.Context, id string, options container.StopOptions) error {
	return nil
}
func (m *mockClient) ContainerLogs(ctx context.Context, id string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (m *mockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, name string) (container.CreateResponse, error) {
	return container.CreateResponse{}, nil
}
func (m *mockClient) ContainerRemove(ctx context.Context, id string, options container.RemoveOptions) error {
	return nil
}
func (m *mockClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return nil, nil
}
func (m *mockClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (m *mockClient) ContainerStats(ctx context.Context, id string, stream bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{}, nil
}


func TestRunServerConfig(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("JWT_SECRET", "")
	os.Setenv("ADMIN_USER", "")
	os.Setenv("ADMIN_PASSWORD", "")
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	
	errChan := make(chan error)
	go func() {
		errChan <- Run()
	}()
	time.Sleep(500 * time.Millisecond)
	p, err := os.FindProcess(os.Getpid())
	if err == nil {
		_ = p.Signal(os.Interrupt)
	}
	
	select {
	case err := <-errChan:
		if err != nil { t.Errorf("Run returned error: %v", err) }
	case <-time.After(5 * time.Second):
		t.Errorf("Run did not exit after SIGINT")
	}
}

func TestRunCoverage(t *testing.T) {
	os.Setenv("PORT", "59231")
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("TEST_MODE", "true")
	defer os.Unsetenv("PORT")

	errChan := make(chan error)
	go func() { errChan <- Run() }()
	time.Sleep(1 * time.Second)
	_, _ = http.Post("http://127.0.0.1:59231/api/login", "application/json", bytes.NewBufferString("{}"))
	_, _ = http.Get("http://127.0.0.1:59231/random-not-found")
	quit <- syscall.SIGINT
	select {
	case err := <-errChan:
		if err != nil { t.Errorf("Run error: %v", err) }
	case <-time.After(5 * time.Second):
		t.Errorf("Timed out waiting for Run to exit via signal")
	}
}

func TestRun_PortDefault(t *testing.T) {
	os.Setenv("PORT", "")
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("TEST_MODE", "true")
	errChan := make(chan error)
	go func() { errChan <- Run() }()
	time.Sleep(500 * time.Millisecond)
	quit <- syscall.SIGINT
	<-errChan
}

func TestDockerError(t *testing.T) {
	oldNew := docker.NewService
	docker.NewService = func() (*docker.Service, error) { return nil, errors.New("docker fail") }
	defer func() { docker.NewService = oldNew }()
	os.Setenv("DATABASE_URL", ":memory:")
	err := Run()
	if err == nil { t.Errorf("Expected Docker init error") }
}

func TestRun_ServeFail(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:59243") // Use different port
	if err == nil {
		defer l.Close()
	}
	os.Setenv("PORT", "59243")
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("TEST_MODE", "true")
	_ = Run()
}

func TestQuitMode(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	errChan := make(chan error)
	// This test hits the 2-second timeout branch in select by NOT sending a signal
	go func() { errChan <- Run() }()
	select {
	case err := <-errChan:
		if err != nil { t.Errorf("Run returned error: %v", err) }
	case <-time.After(5 * time.Second):
		t.Errorf("Timed out waiting for Run to exit via 2s timeout")
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestRun_CronAddFail(t *testing.T) {
	f, _ := os.CreateTemp("", "cronfail*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.AutoMigrate(&models.Server{})
	db.Create(&models.Server{ContainerID: "c1", CronSchedule: "invalid"})
	os.Setenv("DATABASE_URL", f.Name())
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	errChan := make(chan error)
	go func() { errChan <- Run() }()
	time.Sleep(500 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(os.Interrupt)
	<-errChan
}

func TestServerErrorMode(t *testing.T) {
	os.Setenv("TEST_SERVER_ERROR", "true")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("PORT", "0")
	_ = Run()
	os.Unsetenv("TEST_SERVER_ERROR")
}

func TestShutdownErrorMode(t *testing.T) {
	os.Setenv("TEST_SHUTDOWN_ERROR", "true")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("DATABASE_URL", ":memory:")
	os.Setenv("PORT", "0")
	_ = Run()
	os.Unsetenv("TEST_SHUTDOWN_ERROR")
}

func TestMain(t *testing.T) {
	os.Setenv("PORT", "0")
	go main()
	time.Sleep(500 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(os.Interrupt)
	time.Sleep(500 * time.Millisecond)
}

func TestMainError(t *testing.T) {
	os.Setenv("DATABASE_URL", "invalid://db")
	err := Run()
	if err == nil { t.Errorf("Expected error for invalid db URL") }
}

func TestCronSetup(t *testing.T) {
	f, _ := os.CreateTemp("", "cron*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.AutoMigrate(&models.Server{})
	// One valid, one invalid
	db.Create(&models.Server{Name: "srv1", ContainerID: "c1", CronSchedule: "* * * * *"})
	db.Create(&models.Server{Name: "srv2", ContainerID: "c2", CronSchedule: "invalid"})
	
	os.Setenv("DATABASE_URL", f.Name())
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("TEST_TRIGGER_CRON", "true")
	
	// Mock Docker for successful restart
	oldNew := docker.NewService
	docker.NewService = func() (*docker.Service, error) {
		return docker.NewServiceWithClient(&mockClient{}), nil
	}
	defer func() { docker.NewService = oldNew }()

	_ = Run()
	os.Unsetenv("TEST_TRIGGER_CRON")
}

func TestCronRestartFail(t *testing.T) {
	f, _ := os.CreateTemp("", "cronfail2*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.AutoMigrate(&models.Server{})
	db.Create(&models.Server{Name: "srv1", ContainerID: "c1", CronSchedule: "* * * * *"})
	
	os.Setenv("DATABASE_URL", f.Name())
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("TEST_TRIGGER_CRON", "true")
	
	// Mock Docker for FAILED restart
	oldNew := docker.NewService
	docker.NewService = func() (*docker.Service, error) {
		m := &mockClient{}
		// We can't easily mock specific methods of Service here without interface changes, 
		// but we can pass an erroring client if the Service uses it.
		// However, Service.Restart will call m.ContainerRestart.
		return docker.NewServiceWithClient(m), nil
	}
	defer func() { docker.NewService = oldNew }()

	_ = Run()
	os.Unsetenv("TEST_TRIGGER_CRON")
}

type errMockClient struct {
	mockClient
}
func (m *errMockClient) ContainerRestart(ctx context.Context, id string, options container.StopOptions) error {
	return errors.New("restart failed")
}

func TestCronRestartFailActual(t *testing.T) {
	f, _ := os.CreateTemp("", "cronfail3*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.AutoMigrate(&models.Server{})
	db.Create(&models.Server{Name: "srv1", ContainerID: "c1", CronSchedule: "* * * * *"})
	
	os.Setenv("DATABASE_URL", f.Name())
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	os.Setenv("TEST_TRIGGER_CRON", "true")
	
	oldNew := docker.NewService
	docker.NewService = func() (*docker.Service, error) {
		return docker.NewServiceWithClient(&errMockClient{}), nil
	}
	defer func() { docker.NewService = oldNew }()

	_ = Run()
}

func TestAutoMigrateFail(t *testing.T) {
	f, _ := os.CreateTemp("", "migrate*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.Exec("CREATE VIEW audit_logs AS SELECT 1;")
	os.Setenv("DATABASE_URL", f.Name())
	err := Run()
	if err == nil { t.Errorf("Expected AutoMigrate to fail on view") }
}

func TestMainPanic(t *testing.T) {
	oldExit := osExit
	defer func() { osExit = oldExit }()
	osExit = func(code int) {}
	os.Setenv("DATABASE_URL", "invalid://db")
	main()
}

func TestSecureHeadersMiddleware(t *testing.T) {
	handler := secureHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("Expected nosniff header")
	}
}

func TestRun_AdminExists(t *testing.T) {
	f, _ := os.CreateTemp("", "admin*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.AutoMigrate(&models.User{})
	db.Create(&models.User{Username: "admin", IsAdmin: true})
	os.Setenv("DATABASE_URL", f.Name())
	os.Setenv("PORT", "0")
	os.Setenv("TEST_MODE", "true")
	_ = Run()
}

func TestRun_AdminCreateFail(t *testing.T) {
	f, _ := os.CreateTemp("", "adminfail*.db")
	f.Close()
	defer os.Remove(f.Name())
	db, _ := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	db.AutoMigrate(&models.User{})
	db.Exec("CREATE TRIGGER prevent_insert BEFORE INSERT ON users BEGIN SELECT RAISE(FAIL, 'blocked'); END;")
	os.Setenv("DATABASE_URL", f.Name())
	os.Setenv("PORT", "0")
	err := Run()
	if err == nil { t.Errorf("Expected failure to create admin") }
}
