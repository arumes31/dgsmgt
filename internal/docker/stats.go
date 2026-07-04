package docker

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

func (s *Service) DiskUsage(ctx context.Context, containerID string) (sizeRw int64, sizeRootFs int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	inspect, _, err := s.cli.ContainerInspectWithRaw(ctx, containerID, true)
	if err != nil {
		return 0, 0, err
	}
	if inspect.SizeRw != nil {
		sizeRw = *inspect.SizeRw
	}
	if inspect.SizeRootFs != nil {
		sizeRootFs = *inspect.SizeRootFs
	}
	return sizeRw, sizeRootFs, nil
}

// Events subscribes to docker events (used by the watcher for crash
// detection, status pushes and notifications).
type StatsSnapshot struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsed    int64   `json:"mem_used"`
	MemLimit   int64   `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
	NetRx      int64   `json:"net_rx"`
	NetTx      int64   `json:"net_tx"`
	BlockRead  int64   `json:"block_read"`
	BlockWrite int64   `json:"block_write"`
	PIDs       int64   `json:"pids"`
}

func ParseStats(raw []byte) (*StatsSnapshot, error) {
	var v container.StatsResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	snap := &StatsSnapshot{}

	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	if sysDelta > 0 && cpuDelta >= 0 {
		cpus := float64(v.CPUStats.OnlineCPUs)
		if cpus == 0 {
			cpus = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
		}
		if cpus == 0 {
			cpus = 1
		}
		snap.CPUPercent = (cpuDelta / sysDelta) * cpus * 100.0
	}

	// Subtract page cache like `docker stats` does.
	memUsage := v.MemoryStats.Usage
	if cache, ok := v.MemoryStats.Stats["inactive_file"]; ok && cache < memUsage {
		memUsage -= cache
	}
	snap.MemUsed = int64(memUsage)
	snap.MemLimit = int64(v.MemoryStats.Limit)
	if v.MemoryStats.Limit > 0 {
		snap.MemPercent = float64(memUsage) / float64(v.MemoryStats.Limit) * 100.0
	}

	for _, nw := range v.Networks {
		snap.NetRx += int64(nw.RxBytes)
		snap.NetTx += int64(nw.TxBytes)
	}

	for _, bio := range v.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(bio.Op) {
		case "read":
			snap.BlockRead += int64(bio.Value)
		case "write":
			snap.BlockWrite += int64(bio.Value)
		}
	}
	snap.PIDs = int64(v.PidsStats.Current)
	return snap, nil
}

// StatsOnce fetches a single, parsed stats snapshot.
func (s *Service) StatsOnce(ctx context.Context, containerID string) (*StatsSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	resp, err := s.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParseStats(raw)
}
