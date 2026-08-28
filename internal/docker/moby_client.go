package docker

import (
	"context"
	"io"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type mobyClient struct {
	client *client.Client
}

func newMobyClient() (*mobyClient, error) {
	// Do not honor DOCKER_HOST, TLS, or API-version environment overrides in
	// the privileged helper. It may talk only to the mounted local socket.
	cli, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return nil, err
	}
	return &mobyClient{client: cli}, nil
}

func (m *mobyClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	result, err := m.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	return result.Container, err
}

func (m *mobyClient) ContainerStart(ctx context.Context, containerID string) error {
	_, err := m.client.ContainerStart(ctx, containerID, client.ContainerStartOptions{})
	return err
}

func (m *mobyClient) ContainerStop(ctx context.Context, containerID string) error {
	_, err := m.client.ContainerStop(ctx, containerID, client.ContainerStopOptions{})
	return err
}

func (m *mobyClient) ContainerRestart(ctx context.Context, containerID string) error {
	_, err := m.client.ContainerRestart(ctx, containerID, client.ContainerRestartOptions{})
	return err
}

func (m *mobyClient) ContainerLogs(ctx context.Context, containerID, tail string) (io.ReadCloser, error) {
	return m.client.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       tail,
	})
}

func (m *mobyClient) ContainerCreate(
	ctx context.Context,
	name string,
	config *container.Config,
	hostConfig *container.HostConfig,
) (string, error) {
	result, err := m.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:       name,
		Config:     config,
		HostConfig: hostConfig,
	})
	return result.ID, err
}

func (m *mobyClient) ContainerRemove(ctx context.Context, containerID string, force bool) error {
	_, err := m.client.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: force})
	return err
}

func (m *mobyClient) ContainerList(ctx context.Context) ([]container.Summary, error) {
	result, err := m.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	return result.Items, err
}

func (m *mobyClient) ImagePull(ctx context.Context, imageName string) (io.ReadCloser, error) {
	return m.client.ImagePull(ctx, imageName, client.ImagePullOptions{})
}

func (m *mobyClient) ContainerStats(ctx context.Context, containerID string) (io.ReadCloser, error) {
	result, err := m.client.ContainerStats(ctx, containerID, client.ContainerStatsOptions{Stream: true})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}
