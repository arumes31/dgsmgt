package docker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

type mockDockerClient struct {
	inspectFunc func(ctx context.Context, containerID string) (container.InspectResponse, error)
	startFunc   func(ctx context.Context, containerID string) error
	stopFunc    func(ctx context.Context, containerID string) error
	restartFunc func(ctx context.Context, containerID string) error
	logsFunc    func(ctx context.Context, containerID, tail string) (io.ReadCloser, error)
	createFunc  func(ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig) (string, error)
	removeFunc  func(ctx context.Context, containerID string, force bool) error
	listFunc    func(ctx context.Context) ([]container.Summary, error)
	pullFunc    func(ctx context.Context, ref string) (io.ReadCloser, error)
	statsFunc   func(ctx context.Context, containerID string) (io.ReadCloser, error)
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	return m.inspectFunc(ctx, containerID)
}
func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string) error {
	return m.startFunc(ctx, containerID)
}
func (m *mockDockerClient) ContainerStop(ctx context.Context, containerID string) error {
	return m.stopFunc(ctx, containerID)
}
func (m *mockDockerClient) ContainerRestart(ctx context.Context, containerID string) error {
	if m.restartFunc != nil {
		return m.restartFunc(ctx, containerID)
	}
	return nil
}
func (m *mockDockerClient) ContainerLogs(ctx context.Context, containerID, tail string) (io.ReadCloser, error) {
	if m.logsFunc != nil {
		return m.logsFunc(ctx, containerID, tail)
	}
	return io.NopCloser(strings.NewReader("log line")), nil
}
func (m *mockDockerClient) ContainerCreate(ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig) (string, error) {
	return m.createFunc(ctx, name, config, hostConfig)
}
func (m *mockDockerClient) ContainerRemove(ctx context.Context, containerID string, force bool) error {
	return m.removeFunc(ctx, containerID, force)
}
func (m *mockDockerClient) ContainerList(ctx context.Context) ([]container.Summary, error) {
	return m.listFunc(ctx)
}
func (m *mockDockerClient) ImagePull(ctx context.Context, ref string) (io.ReadCloser, error) {
	return m.pullFunc(ctx, ref)
}
func (m *mockDockerClient) ContainerStats(ctx context.Context, containerID string) (io.ReadCloser, error) {
	if m.statsFunc != nil {
		return m.statsFunc(ctx, containerID)
	}
	return io.NopCloser(strings.NewReader("{}")), nil
}

func TestService(t *testing.T) {
	target := "soulmask-server"
	mock := &mockDockerClient{}
	imageName := "registry.example/game@sha256:" + strings.Repeat("a", 64)
	policy, err := NewCreationPolicy([]string{imageName}, []string{"/srv/dgsmgt"})
	if err != nil {
		t.Fatalf("NewCreationPolicy() error = %v", err)
	}
	svc := NewServiceWithClientAndPolicy(mock, policy)

	t.Run("GetStatus", func(t *testing.T) {
		mock.inspectFunc = func(ctx context.Context, containerID string) (container.InspectResponse, error) {
			return container.InspectResponse{
				ID:    "1234567890abcdef",
				State: &container.State{Status: container.StateRunning},
				Name:  "/soulmask-server",
				Config: &container.Config{
					Image: "soulmask:latest",
					Labels: map[string]string{
						managedLabel: "true",
					},
				},
			}, nil
		}

		info, err := svc.GetStatus(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		if info.ID != "1234567890ab" {
			t.Errorf("Expected truncated ID 1234567890ab, got %s", info.ID)
		}
		if info.Status != "running" {
			t.Errorf("Expected status running, got %s", info.Status)
		}
	})

	t.Run("Start", func(t *testing.T) {
		called := false
		mock.startFunc = func(ctx context.Context, containerID string) error {
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
		mock.pullFunc = func(ctx context.Context, ref string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		}
		mock.createFunc = func(ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig) (string, error) {
			return "new-container-id", nil
		}

		id, err := svc.Create(
			context.Background(),
			"test-server",
			imageName,
			[]string{"8080:80"},
			nil,
			[]string{"/srv/dgsmgt:/data:rw"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if id != "new-container-id" {
			t.Errorf("Expected new-container-id, got %s", id)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		called := false
		mock.removeFunc = func(ctx context.Context, containerID string, force bool) error {
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

	t.Run("List", func(t *testing.T) {
		mock.listFunc = func(ctx context.Context) ([]container.Summary, error) {
			return []container.Summary{
				{
					ID:    "1234567890abcdef",
					Image: "alpine",
					State: "running",
					Labels: map[string]string{
						managedLabel: "true",
					},
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
		if list[0].ID != "1234567890ab" {
			t.Errorf("Expected truncated ID 1234567890ab, got %s", list[0].ID)
		}
	})
}

func TestCreationPolicyRejectsUnsafeConfiguration(t *testing.T) {
	imageName := "registry.example/game@sha256:" + strings.Repeat("a", 64)
	policy, err := NewCreationPolicy([]string{imageName}, []string{"/srv/dgsmgt"})
	if err != nil {
		t.Fatalf("NewCreationPolicy() error = %v", err)
	}
	svc := NewServiceWithClientAndPolicy(&mockDockerClient{}, policy)

	tests := []struct {
		name    string
		image   string
		ports   []string
		volumes []string
	}{
		{name: "unlisted image", image: "registry.example/other@sha256:" + strings.Repeat("b", 64)},
		{name: "mutable image", image: "registry.example/game:latest"},
		{name: "privileged host port", image: imageName, ports: []string{"80:80"}},
		{name: "host root bind", image: imageName, volumes: []string{"/:/host"}},
		{name: "parent traversal", image: imageName, volumes: []string{"/srv/dgsmgt/../private:/data"}},
		{name: "unlisted child path", image: imageName, volumes: []string{"/srv/dgsmgt/child:/data"}},
		{name: "propagated bind", image: imageName, volumes: []string{"/srv/dgsmgt/test:/data:rshared"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), "test-server", tt.image, tt.ports, nil, tt.volumes)
			if !errors.Is(err, ErrCreationDenied) {
				t.Fatalf("Create() error = %v, want ErrCreationDenied", err)
			}
		})
	}

	if err := policy.validate(imageName, nil, []string{"/srv/dgsmgt:/data:rw"}); err != nil {
		t.Fatalf("validate() rejected an exact allow-listed bind source: %v", err)
	}
}

func TestNewCreationPolicyRequiresDigestImages(t *testing.T) {
	if _, err := NewCreationPolicy([]string{"alpine:latest"}, nil); err == nil {
		t.Fatal("NewCreationPolicy() accepted mutable image")
	}
}
