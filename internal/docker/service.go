package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type DockerClient interface {
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
}

type ContainerInfo struct {
	ID     string   `json:"id"`
	Names  []string `json:"names"`
	Status string   `json:"status"`
	State  string   `json:"state"`
	Image  string   `json:"image"`
	Uptime string   `json:"uptime"`
	Ports  []string `json:"ports"`
}

type Service struct {
	cli DockerClient
}

const defaultTimeout = 10 * time.Second

func NewService() (*Service, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Service{cli: cli}, nil
}

func NewServiceWithClient(cli DockerClient) *Service {
	return &Service{cli: cli}
}

func (s *Service) GetStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
	// Add timeout to Docker API call
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	inspect, err := s.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	ports := []string{}
	if inspect.NetworkSettings != nil {
		for port, bindings := range inspect.NetworkSettings.Ports {
			for _, binding := range bindings {
				ports = append(ports, fmt.Sprintf("%s:%s->%s", binding.HostIP, binding.HostPort, port))
			}
		}
	}

	return &ContainerInfo{
		ID:     inspect.ID,
		Names:  []string{inspect.Name},
		Status: inspect.State.Status,
		State:  inspect.State.Status,
		Image:  inspect.Config.Image,
		Uptime: inspect.State.StartedAt,
		Ports:  ports,
	}, nil
}

func (s *Service) Start(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

func (s *Service) Stop(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerStop(ctx, containerID, container.StopOptions{})
}

func (s *Service) Restart(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerRestart(ctx, containerID, container.StopOptions{})
}

func (s *Service) Logs(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
	// Logs is a streaming endpoint, but we add initial timeout for connecting/fetching first logs
	// In some implementations, logs follow context cancellation.
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       tail,
	}
	return s.cli.ContainerLogs(ctx, containerID, options)
}

func (s *Service) Stats(ctx context.Context, containerID string) (io.ReadCloser, error) {
	resp, err := s.cli.ContainerStats(ctx, containerID, true)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *Service) Create(ctx context.Context, name string, imageName string, ports []string, env []string, volumes []string) (string, error) {
	// Sanitize name
	if !isValidContainerName(name) {
		return "", fmt.Errorf("invalid container name: %s", name)
	}

	// Docker pull can take longer, use longer timeout
	pullCtx, pullCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer pullCancel()

	// Pull image first
	rc, err := s.cli.ImagePull(pullCtx, imageName, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("pulling image: %w", err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return "", fmt.Errorf("reading pull output: %w", err)
	}

	// Port configuration
	exposedPorts, portBindings, err := nat.ParsePortSpecs(ports)
	if err != nil {
		return "", fmt.Errorf("parsing ports: %w", err)
	}

	config := &container.Config{
		Image:        imageName,
		Env:          env,
		ExposedPorts: exposedPorts,
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        volumes,
	}

	createCtx, createCancel := context.WithTimeout(ctx, defaultTimeout)
	defer createCancel()
	resp, err := s.cli.ContainerCreate(createCtx, config, hostConfig, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	return resp.ID, nil
}

func isValidContainerName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

func (s *Service) Delete(ctx context.Context, containerID string, force bool) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force: force,
	})
}

func (s *Service) List(ctx context.Context) ([]ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	containers, err := s.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	infoList := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		ports := []string{}
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
			} else {
				ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
			}
		}

		infoList = append(infoList, ContainerInfo{
			ID:     c.ID,
			Names:  c.Names,
			Status: c.Status,
			State:  c.State,
			Image:  c.Image,
			Uptime: c.Status, // For list, status often contains uptime string like "Up 2 hours"
			Ports:  ports,
		})
	}

	return infoList, nil
}

