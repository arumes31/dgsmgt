package docker

import (
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func (s *Service) Logs(ctx context.Context, containerID string, tail string, timestamps bool) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       tail,
		Timestamps: timestamps,
	}
	return s.cli.ContainerLogs(ctx, containerID, options)
}

// LogsOnce fetches logs without following (for downloads).
func (s *Service) LogsOnce(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
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

// CreateOpts carries everything needed to create (or recreate) a container.
type CreateOpts struct {
	Name          string
	Image         string
	Ports         []string
	Env           []string
	Volumes       []string
	RestartPolicy string  // no|on-failure|unless-stopped|always
	CPULimit      float64 // cores
	MemoryLimitMB int64
	NetworkMode   string
	StopSignal    string
	StopTimeout   int
	Labels        map[string]string
	SkipPull      bool
}

func (s *Service) Exec(ctx context.Context, containerID string, cmd []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	exec, err := s.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", err
	}
	att, err := s.cli.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", err
	}
	defer att.Close()

	out, err := io.ReadAll(att.Reader)
	if err != nil {
		return "", err
	}
	return string(stripDockerStreamHeaders(out)), nil
}

// AttachConsole attaches to the container's main process with stdin
// (interactive console).
func (s *Service) AttachConsole(ctx context.Context, containerID string) (types.HijackedResponse, error) {
	return s.cli.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
}

// SendStdin writes a line to the container's stdin via attach.
func (s *Service) SendStdin(ctx context.Context, containerID, line string) error {
	att, err := s.cli.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return err
	}
	defer att.Close()
	_, err = att.Conn.Write([]byte(line + "\n"))
	return err
}

// DiskUsage returns the container's writable-layer and rootfs size in bytes.
func stripDockerStreamHeaders(data []byte) []byte {
	var result []byte
	dataLen := len(data)
	for i := 0; i < dataLen; {
		if i+8 > dataLen {
			result = append(result, data[i:]...)
			break
		}
		header := data[i : i+8]
		size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
		start := i + 8
		end := start + size
		if end > dataLen || end < start {
			result = append(result, data[start:]...)
			break
		}
		result = append(result, data[start:end]...)
		i = end
	}
	if len(result) == 0 && dataLen > 0 {
		return data
	}
	return result
}

// ParseStats converts one raw docker stats JSON document into a clean snapshot.
