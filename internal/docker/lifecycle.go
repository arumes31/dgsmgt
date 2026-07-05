package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
)

func (s *Service) Start(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// Stop stops a container honoring a per-server timeout (seconds) and signal.
func (s *Service) Stop(ctx context.Context, containerID string, timeoutSec int, signal string) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec+15)*time.Second)
	defer cancel()
	opts := container.StopOptions{Timeout: &timeoutSec}
	if signal != "" {
		opts.Signal = signal
	}
	return s.cli.ContainerStop(ctx, containerID, opts)
}

func (s *Service) Restart(ctx context.Context, containerID string, timeoutSec int, signal string) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec+25)*time.Second)
	defer cancel()
	opts := container.StopOptions{Timeout: &timeoutSec}
	if signal != "" {
		opts.Signal = signal
	}
	return s.cli.ContainerRestart(ctx, containerID, opts)
}

func (s *Service) Kill(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerKill(ctx, containerID, "SIGKILL")
}

func (s *Service) Pause(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerPause(ctx, containerID)
}

func (s *Service) Unpause(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerUnpause(ctx, containerID)
}

func (s *Service) Rename(ctx context.Context, containerID, newName string) error {
	if !isValidContainerName(newName) {
		return fmt.Errorf("invalid container name: %s", newName)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return s.cli.ContainerRename(ctx, containerID, newName)
}
func (s *Service) buildConfigs(o CreateOpts) (*container.Config, *container.HostConfig, error) {
	exposedPorts, portBindings, err := nat.ParsePortSpecs(o.Ports)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing ports: %w", err)
	}

	labels := map[string]string{ManagedLabel: "true"}
	for k, v := range o.Labels {
		labels[k] = v
	}

	cfg := &container.Config{
		Image:        o.Image,
		Env:          o.Env,
		ExposedPorts: exposedPorts,
		Labels:       labels,
		// Keep stdin open (compose stdin_open) so the console's attach mode
		// can write commands to games that read from stdin.
		OpenStdin: true,
	}
	if len(o.Cmd) > 0 {
		cfg.Cmd = o.Cmd
	}
	if o.StopSignal != "" {
		cfg.StopSignal = o.StopSignal
	}
	if o.StopTimeout > 0 {
		st := o.StopTimeout
		cfg.StopTimeout = &st
	}

	hostCfg := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        o.Volumes,
	}
	if o.RestartPolicy != "" && o.RestartPolicy != "no" {
		hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(o.RestartPolicy)}
	}
	if o.CPULimit > 0 {
		hostCfg.Resources.NanoCPUs = int64(o.CPULimit * 1e9)
	}
	if o.MemoryLimitMB > 0 {
		hostCfg.Resources.Memory = o.MemoryLimitMB * 1024 * 1024
	}
	if o.NetworkMode != "" {
		hostCfg.NetworkMode = container.NetworkMode(o.NetworkMode)
	}
	return cfg, hostCfg, nil
}

// Create pulls the image (unless SkipPull) and creates the container.
func (s *Service) Create(ctx context.Context, o CreateOpts) (string, error) {
	if !isValidContainerName(o.Name) {
		return "", fmt.Errorf("invalid container name: %s", o.Name)
	}

	if !o.SkipPull {
		if err := s.Pull(ctx, o.Image, nil); err != nil {
			return "", err
		}
	}

	cfg, hostCfg, err := s.buildConfigs(o)
	if err != nil {
		return "", err
	}

	createCtx, createCancel := context.WithTimeout(ctx, defaultTimeout)
	defer createCancel()
	resp, err := s.cli.ContainerCreate(createCtx, cfg, hostCfg, nil, nil, o.Name)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	return resp.ID, nil
}

// Pull pulls an image; if progress is non-nil each raw JSON progress line is
// forwarded to it (streamed to the browser during create/redeploy).
func (s *Service) Pull(ctx context.Context, imageName string, progress func(line string)) error {
	pullCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	rc, err := s.cli.ImagePull(pullCtx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	defer func() { _ = rc.Close() }()

	if progress == nil {
		_, err = io.Copy(io.Discard, rc)
		return err
	}
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		progress(scanner.Text())
	}
	return scanner.Err()
}

// Recreate replaces a container with a new one built from opts. Volume binds
// point at host paths so data survives. The old container is kept (renamed
// aside) until the replacement is confirmed, so a failed create or start
// rolls back instead of leaving no container at all. Returns the new ID.
func (s *Service) Recreate(ctx context.Context, oldContainerID string, o CreateOpts, pullLatest bool, progress func(string)) (string, error) {
	wasRunning := false
	oldName := ""
	if inspect, err := s.cli.ContainerInspect(ctx, oldContainerID); err == nil {
		wasRunning = inspect.State != nil && inspect.State.Running
		oldName = strings.TrimPrefix(inspect.Name, "/")
	}

	if pullLatest {
		if err := s.Pull(ctx, o.Image, progress); err != nil {
			return "", err
		}
		o.SkipPull = true
	}

	// Stage: move the old container aside so the new one can take its name.
	// A failed rename must abort — creating would hit a name conflict while
	// rollback has nothing to restore.
	staged := false
	if oldName != "" {
		tmpName := fmt.Sprintf("%s-old-%d", o.Name, time.Now().Unix())
		if err := s.cli.ContainerRename(ctx, oldContainerID, tmpName); err != nil {
			return "", fmt.Errorf("staging old container aside: %w", err)
		}
		staged = true
	}

	// Rollback/cleanup must survive a cancelled request ctx — a client
	// disconnect or timeout can be the very reason Create/Start failed, and the
	// documented "previous container restored" guarantee would otherwise become
	// a silent no-op. Run these ops on a fresh background context.
	rollback := func() {
		if staged {
			rbctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()
			_ = s.cli.ContainerRename(rbctx, oldContainerID, oldName)
		}
	}

	newID, err := s.Create(ctx, o)
	if err != nil {
		rollback()
		return "", err
	}

	if wasRunning {
		// Free the ports held by the old container, then bring up the new one.
		_ = s.cli.ContainerStop(ctx, oldContainerID, container.StopOptions{})
		if err := s.Start(ctx, newID); err != nil {
			rbctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()
			_ = s.Delete(rbctx, newID, true)
			rollback()
			_ = s.cli.ContainerStart(rbctx, oldContainerID, container.StartOptions{})
			return "", fmt.Errorf("new container failed to start; previous container restored: %w", err)
		}
	}

	if err := s.Delete(ctx, oldContainerID, true); err != nil {
		// Non-fatal: the replacement is up; the staged old container lingers
		// under its temporary name until removed manually.
		return newID, nil
	}
	return newID, nil
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
