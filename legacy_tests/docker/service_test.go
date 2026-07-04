package docker

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type mockDockerClient struct {
	inspectFunc func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	startFunc   func(ctx context.Context, containerID string, options container.StartOptions) error
	stopFunc    func(ctx context.Context, containerID string, options container.StopOptions) error
	restartFunc func(ctx context.Context, containerID string, options container.StopOptions) error
	logsFunc    func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	createFunc  func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	removeFunc  func(ctx context.Context, containerID string, options container.RemoveOptions) error
	listFunc    func(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	pullFunc    func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error)
	statsFunc   func(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return m.inspectFunc(ctx, containerID)
}
func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return m.startFunc(ctx, containerID, options)
}
func (m *mockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return m.stopFunc(ctx, containerID, options)
}
func (m *mockDockerClient) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	if m.restartFunc != nil {
		return m.restartFunc(ctx, containerID, options)
	}
	return nil
}
func (m *mockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.logsFunc != nil {
		return m.logsFunc(ctx, containerID, options)
	}
	return io.NopCloser(strings.NewReader("log line")), nil
}
func (m *mockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	return m.createFunc(ctx, config, hostConfig, networkingConfig, platform, containerName)
}
func (m *mockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	return m.removeFunc(ctx, containerID, options)
}
func (m *mockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return m.listFunc(ctx, options)
}
func (m *mockDockerClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	return m.pullFunc(ctx, ref, options)
}
func (m *mockDockerClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	if m.statsFunc != nil {
		return m.statsFunc(ctx, containerID, stream)
	}
	return container.StatsResponseReader{}, nil
}

func TestService(t *testing.T) {
	target := "soulmask-server"
	mock := &mockDockerClient{}
	svc := NewServiceWithClient(mock)

	t.Run("GetStatus", func(t *testing.T) {
		mock.inspectFunc = func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID: "1234567890abcdef",
					State: &types.ContainerState{
						Status: "running",
					},
					Name: "/soulmask-server",
				},
				Config: &container.Config{
					Image: "soulmask:latest",
				},
			}, nil
		}

		info, err := svc.GetStatus(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		if info.ID != "1234567890abcdef" {
			t.Errorf("Expected full ID 1234567890abcdef, got %s", info.ID)
		}
		if info.Status != "running" {
			t.Errorf("Expected status running, got %s", info.Status)
		}
	})

	t.Run("Start", func(t *testing.T) {
		called := false
		mock.startFunc = func(ctx context.Context, containerID string, options container.StartOptions) error {
			called = true
			return nil
		}
		if err := svc.Start(context.Background(), target); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !called {
			t.Error("ContainerStart was not called")
		}
	})

	t.Run("Create", func(t *testing.T) {
		mock.pullFunc = func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		}
		mock.createFunc = func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{ID: "new-container-id"}, nil
		}

		id, err := svc.Create(context.Background(), "test-server", "alpine", []string{"8080:80"}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if id != "new-container-id" {
			t.Errorf("Expected new-container-id, got %s", id)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		called := false
		mock.removeFunc = func(ctx context.Context, containerID string, options container.RemoveOptions) error {
			called = true
			if containerID != target {
				t.Errorf("Expected containerID %s, got %s", target, containerID)
			}
			return nil
		}
		if err := svc.Delete(context.Background(), target, true); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !called {
			t.Error("ContainerRemove was not called")
		}
	})

	t.Run("Stop", func(t *testing.T) {
		called := false
		mock.stopFunc = func(ctx context.Context, containerID string, options container.StopOptions) error {
			called = true
			return nil
		}
		if err := svc.Stop(context.Background(), target); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !called {
			t.Error("ContainerStop was not called")
		}
	})

	t.Run("Restart", func(t *testing.T) {
		called := false
		mock.restartFunc = func(ctx context.Context, containerID string, options container.StopOptions) error {
			called = true
			return nil
		}
		if err := svc.Restart(context.Background(), target); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !called {
			t.Error("ContainerRestart was not called")
		}
	})

	t.Run("Logs", func(t *testing.T) {
		rc, err := svc.Logs(context.Background(), target, "10")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		if string(data) != "log line" {
			t.Errorf("Expected 'log line', got %s", string(data))
		}
	})

	t.Run("Stats", func(t *testing.T) {
		mock.statsFunc = func(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
			return container.StatsResponseReader{Body: io.NopCloser(strings.NewReader("stats"))}, nil
		}
		rc, err := svc.Stats(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		if string(data) != "stats" {
			t.Errorf("Expected 'stats', got %s", string(data))
		}
	})

	t.Run("List", func(t *testing.T) {
		mock.listFunc = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:    "1234567890abcdef",
					Image: "alpine",
					State: "running",
				},
			}, nil
		}

		list, err := svc.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Errorf("Expected 1 container, got %d", len(list))
		}
		if list[0].ID != "1234567890abcdef" {
			t.Errorf("Expected full ID 1234567890abcdef, got %s", list[0].ID)
		}
	})

	t.Run("CreateError", func(t *testing.T) {
		_, err := svc.Create(context.Background(), "invalid/name", "alpine", nil, nil, nil)
		if err == nil {
			t.Error("Expected error for invalid container name")
		}
	})

	t.Run("NewService", func(t *testing.T) {
		_, _ = NewService()
	})

	t.Run("NewServiceError", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "invalid_no_schema")
		_, err := NewService()
		if err == nil {
			t.Error("Expected error with invalid DOCKER_HOST")
		}
	})

	t.Run("GetStatusError", func(t *testing.T) {
		mock.inspectFunc = func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{}, io.EOF
		}
		_, err := svc.GetStatus(context.Background(), target)
		if err == nil {
			t.Error("Expected error from GetStatus")
		}
	})
	
	t.Run("GetStatusPorts", func(t *testing.T) {
		mock.inspectFunc = func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID: "123",
					State: &types.ContainerState{},
				},
				Config: &container.Config{},
				NetworkSettings: &types.NetworkSettings{
					NetworkSettingsBase: types.NetworkSettingsBase{
						Ports: map[nat.Port][]nat.PortBinding{
							"80/tcp": {{HostIP: "0.0.0.0", HostPort: "8080"}},
						},
					},
				},
			}, nil
		}
		info, err := svc.GetStatus(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		if len(info.Ports) == 0 || info.Ports[0] != "0.0.0.0:8080->80/tcp" {
			t.Errorf("Unexpected ports: %v", info.Ports)
		}
	})

	t.Run("StatsError", func(t *testing.T) {
		mock.statsFunc = func(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
			return container.StatsResponseReader{}, io.EOF
		}
		_, err := svc.Stats(context.Background(), target)
		if err == nil {
			t.Error("Expected error from Stats")
		}
	})

	t.Run("CreatePullError", func(t *testing.T) {
		mock.pullFunc = func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
			return nil, io.EOF
		}
		_, err := svc.Create(context.Background(), "test-server", "alpine", nil, nil, nil)
		if err == nil {
			t.Error("Expected error when pulling image fails")
		}
	})

	t.Run("CreateBadPorts", func(t *testing.T) {
		mock.pullFunc = func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		}
		_, err := svc.Create(context.Background(), "test-server", "alpine", []string{"invalid-port-format"}, nil, nil)
		if err == nil {
			t.Error("Expected error with bad ports")
		}
	})

	t.Run("CreateDockerError", func(t *testing.T) {
		mock.pullFunc = func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		}
		mock.createFunc = func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{}, io.EOF
		}
		_, err := svc.Create(context.Background(), "test-server", "alpine", []string{"8080:80"}, nil, nil)
		if err == nil {
			t.Error("Expected error from ContainerCreate")
		}
	})

	t.Run("ListError", func(t *testing.T) {
		mock.listFunc = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return nil, io.EOF
		}
		_, err := svc.List(context.Background())
		if err == nil {
			t.Error("Expected error from ContainerList")
		}
	})

	t.Run("ListPorts", func(t *testing.T) {
		mock.listFunc = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:    "123",
					Ports: []types.Port{
						{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
						{PrivatePort: 443, Type: "tcp"},
					},
				},
			}, nil
		}
		list, err := svc.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(list[0].Ports) != 2 {
			t.Errorf("Expected 2 port mappings, got %d", len(list[0].Ports))
		}
	})

	t.Run("CreateReadPullOutputError", func(t *testing.T) {
		mock.pullFunc = func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(&errReader{}), nil
		}
		_, err := svc.Create(context.Background(), "test-server", "alpine", nil, nil, nil)
		if err == nil {
			t.Error("Expected error reading pull output")
		}
	})

	t.Run("isValidContainerName", func(t *testing.T) {
		if isValidContainerName("") {
			t.Error("Expected false for empty string")
		}
		if !isValidContainerName("valid-name.123_") {
			t.Error("Expected true for valid name")
		}
		if isValidContainerName("invalid@name") {
			t.Error("Expected false for invalid chars")
		}
	})
}

type errReader struct{}

func (e *errReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}
