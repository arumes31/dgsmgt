// Package scheduler runs per-server cron jobs (restart/stop/start/command)
// and scheduled backups. Reload() rebuilds all jobs after config changes.
package scheduler

import (
	"context"
	"fmt"
	"sync"

	"dgsmgt/internal/backup"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"
	"dgsmgt/internal/notify"
	"dgsmgt/internal/rcon"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Scheduler struct {
	db       *gorm.DB
	nodes    *docker.Manager
	backups  *backup.Manager
	notifier *notify.Notifier
	logger   *zap.Logger

	mu     sync.Mutex
	runner *cron.Cron
}

func New(db *gorm.DB, nodes *docker.Manager, backups *backup.Manager, notifier *notify.Notifier, logger *zap.Logger) *Scheduler {
	return &Scheduler{db: db, nodes: nodes, backups: backups, notifier: notifier, logger: logger}
}

// Reload replaces all scheduled jobs from current server config.
func (s *Scheduler) Reload() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runner != nil {
		s.runner.Stop()
	}
	s.runner = cron.New()

	var servers []models.Server
	s.db.Find(&servers)
	for i := range servers {
		srv := servers[i]
		if srv.CronSchedule != "" {
			if _, err := s.runner.AddFunc(srv.CronSchedule, func() { s.runAction(srv.ID) }); err != nil {
				s.logger.Error("cron: invalid schedule", zap.String("server", srv.Name), zap.Error(err))
			}
		}
		if srv.BackupSchedule != "" {
			if _, err := s.runner.AddFunc(srv.BackupSchedule, func() { s.runBackup(srv.ID) }); err != nil {
				s.logger.Error("cron: invalid backup schedule", zap.String("server", srv.Name), zap.Error(err))
			}
		}
	}
	s.runner.Start()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runner != nil {
		s.runner.Stop()
	}
}

// NextRuns returns the next execution times of all jobs (for the UI preview).
func (s *Scheduler) EntryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runner == nil {
		return 0
	}
	return len(s.runner.Entries())
}

func (s *Scheduler) runAction(serverID uint) {
	var srv models.Server
	if err := s.db.First(&srv, serverID).Error; err != nil {
		return
	}
	svc, err := s.nodes.For(srv.NodeID)
	if err != nil {
		return
	}
	ctx := context.Background()

	s.logger.Info("cron: running scheduled action",
		zap.String("server", srv.Name), zap.String("action", srv.CronAction))

	switch srv.CronAction {
	case "stop":
		err = svc.Stop(ctx, srv.ContainerID, srv.StopTimeout, srv.StopSignal)
	case "start":
		err = svc.Start(ctx, srv.ContainerID)
	case "command":
		err = s.runCommand(&srv, svc)
	default: // restart
		err = svc.Restart(ctx, srv.ContainerID, srv.StopTimeout, srv.StopSignal)
	}
	if err != nil {
		s.logger.Error("cron: scheduled action failed", zap.String("server", srv.Name), zap.Error(err))
		s.notifier.Dispatch(notify.Event{
			Type: notify.EventServerDown, ServerID: srv.ID, ServerName: srv.Name,
			Title: fmt.Sprintf("Scheduled %s failed on %s", srv.CronAction, srv.Name),
			Body:  err.Error(), Level: "error",
		})
	}
}

func (s *Scheduler) runCommand(srv *models.Server, svc *docker.Service) error {
	if srv.CronCommand == "" {
		return fmt.Errorf("no cron command configured")
	}
	if srv.ConsoleMode == "rcon" && srv.RconHost != "" {
		_, err := rcon.Send(srv.RconHost, srv.RconPort, srv.RconPassword, srv.CronCommand)
		return err
	}
	return svc.SendStdin(context.Background(), srv.ContainerID, srv.CronCommand)
}

func (s *Scheduler) runBackup(serverID uint) {
	var srv models.Server
	if err := s.db.First(&srv, serverID).Error; err != nil {
		return
	}
	rec, err := s.backups.Run(context.Background(), &srv, "scheduled")
	if err != nil {
		s.notifier.Dispatch(notify.Event{
			Type: notify.EventBackup, ServerID: srv.ID, ServerName: srv.Name,
			Title: fmt.Sprintf("Backup failed for %s", srv.Name),
			Body:  err.Error(), Level: "error",
		})
		return
	}
	s.notifier.Dispatch(notify.Event{
		Type: notify.EventBackup, ServerID: srv.ID, ServerName: srv.Name,
		Title: fmt.Sprintf("Backup completed for %s", srv.Name),
		Body:  fmt.Sprintf("Backup %s (%.1f MB) stored on %s.", rec.FileName, float64(rec.SizeBytes)/1024/1024, rec.Target),
		Level: "success",
	})
}
