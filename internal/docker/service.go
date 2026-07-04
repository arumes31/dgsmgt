package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type DockerClient interface {
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerInspectWithRaw(ctx context.Context, containerID string, getSize bool) (types.ContainerJSON, []byte, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerKill(ctx context.Context, containerID, signal string) error
	ContainerPause(ctx context.Context, containerID string) error
	ContainerUnpause(ctx context.Context, containerID string) error
	ContainerRename(ctx context.Context, containerID, newContainerName string) error
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerAttach(ctx context.Context, containerID string, options container.AttachOptions) (types.HijackedResponse, error)
	ContainerExecCreate(ctx context.Context, containerID string, options container.ExecOptions) (types.IDResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, options container.ExecAttachOptions) (types.HijackedResponse, error)
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, container.PathStat, error)
	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options container.CopyToContainerOptions) error
	ContainerStatPath(ctx context.Context, containerID, path string) (container.PathStat, error)
	ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	NetworkList(ctx context.Context, options network.ListOptions) ([]network.Summary, error)
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	Ping(ctx context.Context) (types.Ping, error)
}

// ManagedLabel marks containers created by this panel.
const ManagedLabel = "dgsmgt.managed"

type HealthInfo struct {
	Status        string `json:"status"` // healthy|unhealthy|starting|none
	FailingStreak int    `json:"failing_streak"`
}

type ContainerInfo struct {
	ID           string     `json:"id"`
	Names        []string   `json:"names"`
	Status       string     `json:"status"`
	State        string     `json:"state"`
	Image        string     `json:"image"`
	Uptime       string     `json:"uptime"`     // humanized, e.g. "3d 4h 12m"
	StartedAt    string     `json:"started_at"` // RFC3339
	Ports        []string   `json:"ports"`
	ExitCode     int        `json:"exit_code"`
	OOMKilled    bool       `json:"oom_killed"`
	RestartCount int        `json:"restart_count"`
	Health       HealthInfo `json:"health"`
	Managed      bool       `json:"managed"`
}

type Service struct {
	cli DockerClient
}

const defaultTimeout = 10 * time.Second

var NewService = func() (*Service, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Service{cli: cli}, nil
}

func NewServiceWithClient(cli DockerClient) *Service {
	return &Service{cli: cli}
}

// HumanizeUptime renders a duration since start as "3d 4h 12m".
func HumanizeUptime(startedAt string) string {
	t, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		return ""
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func (s *Service) GetStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
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

	info := &ContainerInfo{
		ID:        inspect.ID,
		Names:     []string{inspect.Name},
		Status:    inspect.State.Status,
		State:     inspect.State.Status,
		Image:     inspect.Config.Image,
		StartedAt: inspect.State.StartedAt,
		Ports:     ports,
		Health:    HealthInfo{Status: "none"},
	}
	if inspect.State.Running {
		info.Uptime = HumanizeUptime(inspect.State.StartedAt)
	}
	info.ExitCode = inspect.State.ExitCode
	info.OOMKilled = inspect.State.OOMKilled
	info.RestartCount = inspect.RestartCount
	if inspect.State.Health != nil {
		info.Health = HealthInfo{
			Status:        strings.ToLower(inspect.State.Health.Status),
			FailingStreak: inspect.State.Health.FailingStreak,
		}
	}
	if inspect.Config != nil && inspect.Config.Labels != nil {
		info.Managed = inspect.Config.Labels[ManagedLabel] == "true"
	}
	return info, nil
}

// Inspect returns the raw inspect payload (used for mounts, config diffing, adoption).
func (s *Service) Inspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerInspect(ctx, containerID)
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
			ID:      c.ID,
			Names:   c.Names,
			Status:  c.Status,
			State:   c.State,
			Image:   c.Image,
			Uptime:  c.Status,
			Ports:   ports,
			Managed: c.Labels[ManagedLabel] == "true",
		})
	}

	return infoList, nil
}

// Networks lists docker network names for the create form.
func (s *Service) Networks(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	nets, err := s.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(nets))
	for _, n := range nets {
		names = append(names, n.Name)
	}
	return names, nil
}

// EnsureNetwork creates a bridge network if it doesn't exist (stacks).
func (s *Service) EnsureNetwork(ctx context.Context, name string) error {
	nets, err := s.Networks(ctx)
	if err != nil {
		return err
	}
	for _, n := range nets {
		if n == name {
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	_, err = s.cli.NetworkCreate(ctx, name, network.CreateOptions{Driver: "bridge"})
	return err
}

// Exec runs a command in the container and returns combined output.
func (s *Service) Events(ctx context.Context) (<-chan events.Message, <-chan error) {
	return s.cli.Events(ctx, events.ListOptions{})
}

func (s *Service) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.cli.Ping(ctx)
	return err
}

// stripDockerStreamHeaders removes the 8-byte multiplexing headers from a
// docker stream buffer.
