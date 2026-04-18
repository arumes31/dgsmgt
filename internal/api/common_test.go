package api

import (
	"context"
	"dgsmgt/internal/models"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/glebarez/sqlite"
	"github.com/go-playground/validator/v10"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"gorm.io/gorm"
	"reflect"
)

type mockFieldLevel struct {
	validator.FieldLevel
	value string
}

func (m *mockFieldLevel) Field() reflect.Value {
	return reflect.ValueOf(m.value)
}

// mockClient implements docker.DockerClient
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
	logsReadErr error
	statsChan   chan []byte
}

type errorReader struct{}

func (e errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (e errorReader) Close() error { return nil }

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
	if m.logsReadErr != nil { return errorReader{}, nil }
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
	if m.pullErr != nil { return nil, m.pullErr }
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

type errorMockClient struct {
	mockClient
}

func (m *errorMockClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return errors.New("start failed")
}
func (m *errorMockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	return container.CreateResponse{}, errors.New("create failed")
}
func (m *errorMockClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	return errors.New("remove failed")
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	err = db.AutoMigrate(&models.User{}, &models.Server{}, &models.UserServer{}, &models.AuditLog{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	return db
}

func setupFailDB(t *testing.T, model interface{}) *gorm.DB {
	db := setupTestDB(t)
	err := db.Migrator().DropTable(model)
	if err != nil {
		t.Fatalf("failed to drop table: %v", err)
	}
	return db
}

func setupFailOpDB(t *testing.T, operation string) *gorm.DB {
	db := setupTestDB(t)
	err := errors.New("forced " + operation + " error")
	switch operation {
	case "create":
		_ = db.Callback().Create().Before("gorm:create").Register("fail_create", func(d *gorm.DB) { d.Error = err })
	case "update":
		_ = db.Callback().Update().Before("gorm:update").Register("fail_update", func(d *gorm.DB) { d.Error = err })
	case "delete":
		_ = db.Callback().Delete().Before("gorm:delete").Register("fail_delete", func(d *gorm.DB) { d.Error = err })
	}
	return db
}

