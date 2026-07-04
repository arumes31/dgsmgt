// Package monitor runs background workers: metric sampling, docker event
// watching (up/down/crash notifications + panel-side auto-restart with
// backoff), log tailing for regex alerts and optional log persistence.
package monitor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"dgsmgt/internal/config"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"
	"dgsmgt/internal/notify"

	"github.com/docker/docker/api/types/events"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Monitor struct {
	db       *gorm.DB
	nodes    *docker.Manager
	cfg      *config.Config
	logger   *zap.Logger
	notifier *notify.Notifier

	mu          sync.Mutex
	crashCounts map[uint][]time.Time // serverID -> recent crash times
	tailers     map[uint]context.CancelFunc
	alertLast   map[string]time.Time // serverID:pattern -> last alert
}

func New(db *gorm.DB, nodes *docker.Manager, cfg *config.Config, logger *zap.Logger, notifier *notify.Notifier) *Monitor {
	_ = os.MkdirAll(cfg.LogPath, 0750)
	return &Monitor{
		db:          db,
		nodes:       nodes,
		cfg:         cfg,
		logger:      logger,
		notifier:    notifier,
		crashCounts: make(map[uint][]time.Time),
		tailers:     make(map[uint]context.CancelFunc),
		alertLast:   make(map[string]time.Time),
	}
}

// Start launches all background loops; they stop when ctx is cancelled.
func (m *Monitor) Start(ctx context.Context) {
	go m.sampleLoop(ctx)
	go m.pruneLoop(ctx)
	go m.reconcileTailersLoop(ctx)
	for _, nodeID := range m.nodes.NodeIDs() {
		go m.watchEvents(ctx, nodeID)
	}
}

// WatchNode starts event watching for a newly registered node.
func (m *Monitor) WatchNode(ctx context.Context, nodeID uint) {
	go m.watchEvents(ctx, nodeID)
}

// ---- Metrics sampling -----------------------------------------------------

func (m *Monitor) sampleLoop(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.MetricInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sampleAll(ctx)
		}
	}
}

func (m *Monitor) sampleAll(ctx context.Context) {
	var servers []models.Server
	if err := m.db.Find(&servers).Error; err != nil {
		return
	}
	now := time.Now()
	for _, s := range servers {
		svc, err := m.nodes.For(s.NodeID)
		if err != nil || s.ContainerID == "" {
			continue
		}
		sample := models.MetricSample{ServerID: s.ID, At: now}
		if info, err := svc.GetStatus(ctx, s.ContainerID); err == nil && info.State == "running" {
			sample.Running = true
			if snap, err := svc.StatsOnce(ctx, s.ContainerID); err == nil {
				sample.CPUPercent = snap.CPUPercent
				sample.MemUsed = snap.MemUsed
				sample.MemLimit = snap.MemLimit
				sample.NetRx = snap.NetRx
				sample.NetTx = snap.NetTx
			}
		}
		m.db.Create(&sample)
	}
}

func (m *Monitor) pruneLoop(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -m.cfg.MetricRetentionDays)
			m.db.Where("at < ?", cutoff).Delete(&models.MetricSample{})
			auditCutoff := time.Now().AddDate(0, 0, -m.cfg.AuditRetentionDays)
			m.db.Unscoped().Where("created_at < ?", auditCutoff).Delete(&models.AuditLog{})
			// Expired temporary access grants
			m.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).Delete(&models.UserServer{})
			// Expired sessions
			m.db.Unscoped().Where("expires_at < ?", time.Now().AddDate(0, 0, -7)).Delete(&models.Session{})
		}
	}
}

// ---- Docker events: notifications + crash auto-restart ---------------------

func (m *Monitor) watchEvents(ctx context.Context, nodeID uint) {
	for {
		if ctx.Err() != nil {
			return
		}
		svc, err := m.nodes.For(nodeID)
		if err != nil {
			time.Sleep(30 * time.Second)
			continue
		}
		msgs, errs := svc.Events(ctx)
	inner:
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errs:
				if err != nil && err != io.EOF {
					m.logger.Warn("docker events stream error", zap.Uint("node", nodeID), zap.Error(err))
				}
				time.Sleep(5 * time.Second)
				break inner
			case msg := <-msgs:
				m.handleEvent(ctx, nodeID, msg)
			}
		}
	}
}

func (m *Monitor) handleEvent(ctx context.Context, nodeID uint, msg events.Message) {
	if msg.Type != events.ContainerEventType {
		return
	}
	action := string(msg.Action)
	if action != "die" && action != "start" && action != "oom" {
		return
	}

	var server models.Server
	if err := m.db.Where("container_id LIKE ? AND node_id = ?", msg.Actor.ID[:12]+"%", nodeID).
		First(&server).Error; err != nil {
		return // not a managed server
	}

	switch action {
	case "start":
		m.notifier.Dispatch(notify.Event{
			Type: notify.EventServerUp, ServerID: server.ID, ServerName: server.Name,
			Title: fmt.Sprintf("%s is up", server.Name),
			Body:  "Container started.", Level: "success",
		})
	case "oom":
		m.notifier.Dispatch(notify.Event{
			Type: notify.EventServerCrash, ServerID: server.ID, ServerName: server.Name,
			Title: fmt.Sprintf("%s was OOM-killed", server.Name),
			Body:  "The container ran out of memory. Consider raising its memory limit.", Level: "error",
		})
	case "die":
		exitCode := msg.Actor.Attributes["exitCode"]
		if exitCode == "0" {
			m.notifier.Dispatch(notify.Event{
				Type: notify.EventServerDown, ServerID: server.ID, ServerName: server.Name,
				Title: fmt.Sprintf("%s stopped", server.Name),
				Body:  "Container exited cleanly.", Level: "warn",
			})
			return
		}
		m.notifier.Dispatch(notify.Event{
			Type: notify.EventServerCrash, ServerID: server.ID, ServerName: server.Name,
			Title: fmt.Sprintf("%s crashed", server.Name),
			Body:  fmt.Sprintf("Container exited with code %s.", exitCode), Level: "error",
		})
		if server.AutoRestart {
			m.maybeRestart(ctx, &server)
		}
	}
}

// maybeRestart restarts a crashed server unless it is crash-looping
// (more than 5 crashes in 10 minutes).
func (m *Monitor) maybeRestart(ctx context.Context, server *models.Server) {
	m.mu.Lock()
	now := time.Now()
	recent := []time.Time{}
	for _, t := range m.crashCounts[server.ID] {
		if now.Sub(t) < 10*time.Minute {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	m.crashCounts[server.ID] = recent
	count := len(recent)
	m.mu.Unlock()

	if count > 5 {
		m.notifier.Dispatch(notify.Event{
			Type: notify.EventServerCrash, ServerID: server.ID, ServerName: server.Name,
			Title: fmt.Sprintf("%s is crash-looping", server.Name),
			Body:  "Auto-restart suspended after 5 crashes in 10 minutes.", Level: "error",
		})
		return
	}

	backoff := time.Duration(count*count) * time.Second
	m.logger.Info("auto-restarting crashed server",
		zap.String("server", server.Name), zap.Duration("backoff", backoff))
	time.Sleep(backoff)

	svc, err := m.nodes.For(server.NodeID)
	if err != nil {
		return
	}
	if err := svc.Start(ctx, server.ContainerID); err != nil {
		m.logger.Error("auto-restart failed", zap.String("server", server.Name), zap.Error(err))
	}
}

// ---- Log tailing: alerts + persistence -------------------------------------

func (m *Monitor) reconcileTailersLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	m.reconcileTailers(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.reconcileTailers(ctx)
		}
	}
}

func (m *Monitor) reconcileTailers(ctx context.Context) {
	var servers []models.Server
	if err := m.db.Find(&servers).Error; err != nil {
		return
	}

	wanted := map[uint]models.Server{}
	for _, s := range servers {
		var alertCount int64
		m.db.Model(&models.LogAlert{}).Where("server_id = ? AND enabled = ?", s.ID, true).Count(&alertCount)
		if s.PersistLogs || alertCount > 0 {
			wanted[s.ID] = s
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, cancel := range m.tailers {
		if _, ok := wanted[id]; !ok {
			cancel()
			delete(m.tailers, id)
		}
	}
	for id, s := range wanted {
		if _, running := m.tailers[id]; running {
			continue
		}
		tctx, cancel := context.WithCancel(ctx)
		m.tailers[id] = cancel
		go m.tail(tctx, s)
	}
}

func (m *Monitor) tail(ctx context.Context, server models.Server) {
	svc, err := m.nodes.For(server.NodeID)
	if err != nil {
		return
	}
	reader, err := svc.Logs(ctx, server.ContainerID, "0", false)
	if err != nil {
		return
	}
	defer func() { _ = reader.Close() }()

	var logFile *os.File
	if server.PersistLogs {
		p := filepath.Join(m.cfg.LogPath, fmt.Sprintf("%s.log", server.Name))
		logFile, _ = os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640) // #nosec G304
		if logFile != nil {
			defer func() { _ = logFile.Close() }()
			// naive rotation: cap at 50MB
			if fi, err := logFile.Stat(); err == nil && fi.Size() > 50*1024*1024 {
				_ = logFile.Close()
				_ = os.Rename(p, p+".1")
				logFile, _ = os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640) // #nosec G304
			}
		}
	}

	var alerts []models.LogAlert
	m.db.Where("server_id = ? AND enabled = ?", server.ID, true).Find(&alerts)
	patterns := map[string]*regexp.Regexp{}
	for _, a := range alerts {
		if re, err := regexp.Compile(a.Pattern); err == nil {
			patterns[a.Pattern] = re
		}
	}

	buf := make([]byte, 8192)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			clean := stripHeaders(buf[:n])
			if logFile != nil {
				_, _ = logFile.Write(clean)
			}
			line := string(clean)
			for pat, re := range patterns {
				if re.MatchString(line) {
					m.fireLogAlert(server, pat, line)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (m *Monitor) fireLogAlert(server models.Server, pattern, line string) {
	key := fmt.Sprintf("%d:%s", server.ID, pattern)
	m.mu.Lock()
	last, ok := m.alertLast[key]
	if ok && time.Since(last) < 5*time.Minute {
		m.mu.Unlock()
		return
	}
	m.alertLast[key] = time.Now()
	m.mu.Unlock()

	snippet := line
	if len(snippet) > 300 {
		snippet = snippet[:300] + "..."
	}
	m.notifier.Dispatch(notify.Event{
		Type: notify.EventLogAlert, ServerID: server.ID, ServerName: server.Name,
		Title: fmt.Sprintf("Log alert on %s", server.Name),
		Body:  fmt.Sprintf("Pattern `%s` matched:\n%s", pattern, strings.TrimSpace(snippet)),
		Level: "warn",
	})
}

// stripHeaders removes docker stream multiplexing headers.
func stripHeaders(data []byte) []byte {
	var result []byte
	for i := 0; i < len(data); {
		if i+8 > len(data) {
			result = append(result, data[i:]...)
			break
		}
		size := int(data[i+4])<<24 | int(data[i+5])<<16 | int(data[i+6])<<8 | int(data[i+7])
		start := i + 8
		end := start + size
		if end > len(data) || end < start {
			result = append(result, data[start:]...)
			break
		}
		result = append(result, data[start:end]...)
		i = end
	}
	if len(result) == 0 && len(data) > 0 {
		return data
	}
	return result
}
