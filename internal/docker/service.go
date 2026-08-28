package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/moby/moby/api/types/container"
	mobynetwork "github.com/moby/moby/api/types/network"
)

type DockerClient interface {
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerStart(ctx context.Context, containerID string) error
	ContainerStop(ctx context.Context, containerID string) error
	ContainerRestart(ctx context.Context, containerID string) error
	ContainerLogs(ctx context.Context, containerID, tail string) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, name string, config *container.Config, hostConfig *container.HostConfig) (string, error)
	ContainerRemove(ctx context.Context, containerID string, force bool) error
	ContainerList(ctx context.Context) ([]container.Summary, error)
	ImagePull(ctx context.Context, imageName string) (io.ReadCloser, error)
	ContainerStats(ctx context.Context, containerID string) (io.ReadCloser, error)
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
	cli            DockerClient
	remote         *remoteClient
	creationPolicy CreationPolicy
}

const defaultTimeout = 10 * time.Second

var (
	ErrCreationDenied     = errors.New("container creation denied by policy")
	ErrUnmanagedContainer = errors.New("container is not managed by dgsmgt")
)

const managedLabel = "com.dgsmgt.managed"

// CreationPolicy constrains the Docker authority exposed through the web API.
// Images must be exact digest references and host binds must remain beneath an
// explicitly configured source. Exact sources avoid symlink-based escapes that
// cannot be resolved safely from inside the Docker helper container. An empty
// policy disables container creation.
type CreationPolicy struct {
	allowedImages        map[string]struct{}
	allowedVolumeSources []string
	minHostPort          int
	maxHostPort          int
}

func NewCreationPolicy(images, volumeRoots []string) (CreationPolicy, error) {
	policy := CreationPolicy{
		allowedImages: make(map[string]struct{}, len(images)),
		minHostPort:   1024,
		maxHostPort:   65535,
	}

	for _, imageName := range images {
		imageName = strings.TrimSpace(imageName)
		if !isDigestReference(imageName) {
			return CreationPolicy{}, fmt.Errorf("allowed image %q must use an immutable sha256 digest", imageName)
		}
		policy.allowedImages[imageName] = struct{}{}
	}

	for _, root := range volumeRoots {
		root = path.Clean(strings.TrimSpace(root))
		if !path.IsAbs(root) || root == "/" {
			return CreationPolicy{}, fmt.Errorf("allowed volume root %q must be an absolute non-root path", root)
		}
		policy.allowedVolumeSources = append(policy.allowedVolumeSources, root)
	}

	return policy, nil
}

func NewService() (*Service, error) {
	return NewServiceWithPolicy(CreationPolicy{allowedImages: map[string]struct{}{}})
}

func NewServiceWithPolicy(policy CreationPolicy) (*Service, error) {
	cli, err := newMobyClient()
	if err != nil {
		return nil, err
	}
	return &Service{cli: cli, creationPolicy: policy}, nil
}

func NewServiceWithClient(cli DockerClient) *Service {
	return &Service{cli: cli, creationPolicy: CreationPolicy{allowedImages: map[string]struct{}{}}}
}

func NewServiceWithClientAndPolicy(cli DockerClient, policy CreationPolicy) *Service {
	return &Service{cli: cli, creationPolicy: policy}
}

func (s *Service) GetStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
	if s.remote != nil {
		return s.remote.getStatus(ctx, containerID)
	}

	// Add timeout to Docker API call
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	inspect, err := s.managedInspect(ctx, containerID)
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
		ID:     shortID(inspect.ID),
		Names:  []string{inspect.Name},
		Status: string(inspect.State.Status),
		State:  string(inspect.State.Status),
		Image:  inspect.Config.Image,
		Uptime: inspect.State.StartedAt,
		Ports:  ports,
	}, nil
}

func (s *Service) Start(ctx context.Context, containerID string) error {
	if s.remote != nil {
		return s.remote.action(ctx, "start", containerID)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	if _, err := s.managedInspect(ctx, containerID); err != nil {
		return err
	}
	return s.cli.ContainerStart(ctx, containerID)
}

func (s *Service) Stop(ctx context.Context, containerID string) error {
	if s.remote != nil {
		return s.remote.action(ctx, "stop", containerID)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	if _, err := s.managedInspect(ctx, containerID); err != nil {
		return err
	}
	return s.cli.ContainerStop(ctx, containerID)
}

func (s *Service) Restart(ctx context.Context, containerID string) error {
	if s.remote != nil {
		return s.remote.action(ctx, "restart", containerID)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	if _, err := s.managedInspect(ctx, containerID); err != nil {
		return err
	}
	return s.cli.ContainerRestart(ctx, containerID)
}

func (s *Service) Logs(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
	if s.remote != nil {
		return s.remote.stream(ctx, "logs", containerID, tail)
	}
	if _, err := s.managedInspect(ctx, containerID); err != nil {
		return nil, err
	}
	// Logs is a streaming endpoint, but we add initial timeout for connecting/fetching first logs
	// In some implementations, logs follow context cancellation.
	return s.cli.ContainerLogs(ctx, containerID, tail)
}

func (s *Service) Stats(ctx context.Context, containerID string) (io.ReadCloser, error) {
	if s.remote != nil {
		return s.remote.stream(ctx, "stats", containerID, "")
	}
	if _, err := s.managedInspect(ctx, containerID); err != nil {
		return nil, err
	}
	return s.cli.ContainerStats(ctx, containerID)
}

func (s *Service) Create(ctx context.Context, name string, imageName string, ports []string, env []string, volumes []string) (string, error) {
	if s.remote != nil {
		return s.remote.create(ctx, createRequest{
			Name:    name,
			Image:   imageName,
			Ports:   ports,
			Env:     env,
			Volumes: volumes,
		})
	}

	// Sanitize name
	if !isValidContainerName(name) {
		return "", fmt.Errorf("invalid container name: %s", name)
	}
	if err := s.creationPolicy.validate(imageName, ports, volumes); err != nil {
		return "", err
	}

	// Docker pull can take longer, use longer timeout
	pullCtx, pullCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer pullCancel()

	// Pull image first
	rc, err := s.cli.ImagePull(pullCtx, imageName)
	if err != nil {
		return "", fmt.Errorf("pulling image: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return "", fmt.Errorf("reading pull output: %w", err)
	}

	// Port configuration
	natExposedPorts, natPortBindings, err := nat.ParsePortSpecs(ports)
	if err != nil {
		return "", fmt.Errorf("parsing ports: %w", err)
	}
	exposedPorts, portBindings, err := toMobyPorts(natExposedPorts, natPortBindings)
	if err != nil {
		return "", err
	}

	config := &container.Config{
		Image:        imageName,
		Env:          env,
		ExposedPorts: exposedPorts,
		Labels: map[string]string{
			managedLabel: "true",
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        volumes,
	}

	createCtx, createCancel := context.WithTimeout(ctx, defaultTimeout)
	defer createCancel()
	containerID, err := s.cli.ContainerCreate(createCtx, name, config, hostConfig)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	return containerID, nil
}

func toMobyPorts(
	exposed nat.PortSet,
	bindings nat.PortMap,
) (mobynetwork.PortSet, mobynetwork.PortMap, error) {
	mobyExposed := make(mobynetwork.PortSet, len(exposed))
	mobyBindings := make(mobynetwork.PortMap, len(bindings))
	for port := range exposed {
		mobyPort, err := mobynetwork.ParsePort(port.Port() + "/" + port.Proto())
		if err != nil {
			return nil, nil, fmt.Errorf("converting exposed port: %w", err)
		}
		mobyExposed[mobyPort] = struct{}{}
	}
	for port, portBindings := range bindings {
		mobyPort, err := mobynetwork.ParsePort(port.Port() + "/" + port.Proto())
		if err != nil {
			return nil, nil, fmt.Errorf("converting port binding: %w", err)
		}
		converted := make([]mobynetwork.PortBinding, 0, len(portBindings))
		for _, binding := range portBindings {
			var hostIP netip.Addr
			if binding.HostIP != "" {
				hostIP, err = netip.ParseAddr(binding.HostIP)
				if err != nil {
					return nil, nil, fmt.Errorf("parsing host IP %q: %w", binding.HostIP, err)
				}
			}
			converted = append(converted, mobynetwork.PortBinding{
				HostIP:   hostIP,
				HostPort: binding.HostPort,
			})
		}
		mobyBindings[mobyPort] = converted
	}
	return mobyExposed, mobyBindings, nil
}

func (p CreationPolicy) validate(imageName string, ports, volumes []string) error {
	if _, allowed := p.allowedImages[imageName]; !allowed {
		return fmt.Errorf("%w: image is not allow-listed", ErrCreationDenied)
	}

	_, bindings, err := nat.ParsePortSpecs(ports)
	if err != nil {
		return fmt.Errorf("%w: invalid port mapping: %v", ErrCreationDenied, err)
	}
	for _, portBindings := range bindings {
		for _, binding := range portBindings {
			if binding.HostPort == "" {
				continue
			}
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil || hostPort < p.minHostPort || hostPort > p.maxHostPort {
				return fmt.Errorf(
					"%w: host ports must be between %d and %d",
					ErrCreationDenied,
					p.minHostPort,
					p.maxHostPort,
				)
			}
		}
	}

	for _, volume := range volumes {
		if err := p.validateVolume(volume); err != nil {
			return err
		}
	}

	return nil
}

func (p CreationPolicy) validateVolume(volume string) error {
	parts := strings.Split(volume, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("%w: invalid volume mapping", ErrCreationDenied)
	}

	source := path.Clean(parts[0])
	target := path.Clean(parts[1])
	if !path.IsAbs(source) || !path.IsAbs(target) {
		return fmt.Errorf("%w: volume paths must be absolute", ErrCreationDenied)
	}
	if len(parts) == 3 && !validVolumeOptions(parts[2]) {
		return fmt.Errorf("%w: unsafe volume options", ErrCreationDenied)
	}

	for _, allowedSource := range p.allowedVolumeSources {
		if source == allowedSource {
			return nil
		}
	}
	return fmt.Errorf("%w: host volume source is not allow-listed", ErrCreationDenied)
}

func validVolumeOptions(options string) bool {
	if options == "" {
		return true
	}
	for _, option := range strings.Split(options, ",") {
		switch option {
		case "ro", "rw", "z", "Z":
		default:
			return false
		}
	}
	return true
}

func isDigestReference(imageName string) bool {
	name, digest, found := strings.Cut(imageName, "@sha256:")
	if !found || name == "" || len(digest) != 64 {
		return false
	}
	for _, character := range digest {
		isDigit := character >= '0' && character <= '9'
		isLowerHex := character >= 'a' && character <= 'f'
		if !isDigit && !isLowerHex {
			return false
		}
	}
	return true
}

func isValidContainerName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '_', character == '-', character == '.':
		default:
			return false
		}
	}
	return true
}

func (s *Service) Delete(ctx context.Context, containerID string, force bool) error {
	if s.remote != nil {
		return s.remote.delete(ctx, containerID, force)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	if _, err := s.managedInspect(ctx, containerID); err != nil {
		return err
	}
	return s.cli.ContainerRemove(ctx, containerID, force)
}

func (s *Service) List(ctx context.Context) ([]ContainerInfo, error) {
	if s.remote != nil {
		return s.remote.list(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	containers, err := s.cli.ContainerList(ctx)
	if err != nil {
		return nil, err
	}

	infoList := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		if c.Labels[managedLabel] != "true" {
			continue
		}
		ports := []string{}
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
			} else {
				ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
			}
		}

		infoList = append(infoList, ContainerInfo{
			ID:     shortID(c.ID),
			Names:  c.Names,
			Status: c.Status,
			State:  string(c.State),
			Image:  c.Image,
			Uptime: c.Status, // For list, status often contains uptime string like "Up 2 hours"
			Ports:  ports,
		})
	}

	return infoList, nil
}

func (s *Service) managedInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	inspect, err := s.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return container.InspectResponse{}, err
	}
	if inspect.Config == nil || inspect.Config.Labels[managedLabel] != "true" {
		return container.InspectResponse{}, ErrUnmanagedContainer
	}
	return inspect, nil
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
