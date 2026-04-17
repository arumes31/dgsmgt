package api

import (
	"context"
	"io"
	"strings"
	"testing"

	"dgsmgt/internal/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/zap"
)

type mockClient struct {
}

func (m *mockClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    "1234567890abcdef",
			State: &types.ContainerState{Status: "running"},
		},
		Config: &container.Config{Image: "img"},
	}, nil
}

func (m *mockClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return nil
}
func (m *mockClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return nil
}
func (m *mockClient) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	return nil
}
func (m *mockClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("log line")), nil
}
func (m *mockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	return container.CreateResponse{ID: "new-id"}, nil
}
func (m *mockClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	return nil
}
func (m *mockClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return []types.Container{}, nil
}
func (m *mockClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{}, nil
}

func TestAPIHandlers(t *testing.T) {
	mockCli := &mockClient{}
	svc := docker.NewServiceWithClient(mockCli)
	logger, _ := zap.NewDevelopment()
	api := NewAPI(svc, nil, "secret", []string{"*"}, logger)

	if api == nil {
		t.Fatal("Failed to create API")
	}
}
