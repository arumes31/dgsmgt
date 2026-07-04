package api

import (
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"
	"encoding/json"
)

type serverInput struct {
	Name            string   `json:"name" validate:"required,min=3,max=64"`
	Image           string   `json:"image" validate:"required"`
	Ports           []string `json:"ports"`
	Env             []string `json:"env"`
	Volumes         []string `json:"volumes"`
	NodeID          uint     `json:"node_id"`
	Folder          string   `json:"folder" validate:"omitempty,max=64"`
	Icon            string   `json:"icon" validate:"omitempty,max=32"`
	DependsOn       string   `json:"depends_on"`
	StackID         uint     `json:"stack_id"`
	CronSchedule    string   `json:"cron_schedule" validate:"omitempty,cron"`
	CronAction      string   `json:"cron_action" validate:"omitempty,oneof=restart stop start command"`
	CronCommand     string   `json:"cron_command"`
	BackupSchedule  string   `json:"backup_schedule" validate:"omitempty,cron"`
	BackupRetention int      `json:"backup_retention" validate:"omitempty,min=0,max=365"`
	BackupTarget    string   `json:"backup_target" validate:"omitempty,oneof=local s3 sftp"`
	StopTimeout     int      `json:"stop_timeout" validate:"omitempty,min=1,max=600"`
	StopSignal      string   `json:"stop_signal"`
	RestartPolicy   string   `json:"restart_policy" validate:"omitempty,oneof=no on-failure unless-stopped always"`
	CPULimit        float64  `json:"cpu_limit" validate:"omitempty,min=0,max=256"`
	MemoryLimitMB   int64    `json:"memory_limit_mb" validate:"omitempty,min=0"`
	NetworkMode     string   `json:"network_mode"`
	AutoRestart     bool     `json:"auto_restart"`
	AutoUpdate      bool     `json:"auto_update"`
	PersistLogs     bool     `json:"persist_logs"`
	HealthCheckType string   `json:"health_check_type" validate:"omitempty,oneof=none docker tcp"`
	HealthCheckPort int      `json:"health_check_port"`
	RconHost        string   `json:"rcon_host"`
	RconPort        int      `json:"rcon_port"`
	RconPassword    string   `json:"rcon_password"`
	ConsoleMode     string   `json:"console_mode" validate:"omitempty,oneof=attach rcon none"`
}

type containerConfig struct {
	Ports   []string `json:"ports"`
	Env     []string `json:"env"`
	Volumes []string `json:"volumes"`
}

func (a *API) createOptsFor(s *models.Server) docker.CreateOpts {
	var cc containerConfig
	_ = json.Unmarshal([]byte(s.ConfigJSON), &cc)
	return docker.CreateOpts{
		Name:          s.Name,
		Image:         s.Image,
		Ports:         cc.Ports,
		Env:           cc.Env,
		Volumes:       cc.Volumes,
		RestartPolicy: s.RestartPolicy,
		CPULimit:      s.CPULimit,
		MemoryLimitMB: s.MemoryLimitMB,
		NetworkMode:   s.NetworkMode,
		StopSignal:    s.StopSignal,
		StopTimeout:   s.StopTimeout,
	}
}

func applyInput(s *models.Server, in *serverInput) {
	cc := containerConfig{Ports: in.Ports, Env: in.Env, Volumes: in.Volumes}
	b, _ := json.Marshal(cc)
	s.Name = in.Name
	s.Image = in.Image
	s.ConfigJSON = string(b)
	s.NodeID = in.NodeID
	s.Folder = in.Folder
	s.Icon = in.Icon
	s.DependsOn = in.DependsOn
	s.StackID = in.StackID
	s.CronSchedule = in.CronSchedule
	if in.CronAction != "" {
		s.CronAction = in.CronAction
	}
	s.CronCommand = in.CronCommand
	s.BackupSchedule = in.BackupSchedule
	if in.BackupRetention > 0 {
		s.BackupRetention = in.BackupRetention
	}
	if in.BackupTarget != "" {
		s.BackupTarget = in.BackupTarget
	}
	if in.StopTimeout > 0 {
		s.StopTimeout = in.StopTimeout
	}
	s.StopSignal = in.StopSignal
	if in.RestartPolicy != "" {
		s.RestartPolicy = in.RestartPolicy
	}
	s.CPULimit = in.CPULimit
	s.MemoryLimitMB = in.MemoryLimitMB
	s.NetworkMode = in.NetworkMode
	s.AutoRestart = in.AutoRestart
	s.AutoUpdate = in.AutoUpdate
	s.PersistLogs = in.PersistLogs
	if in.HealthCheckType != "" {
		s.HealthCheckType = in.HealthCheckType
	}
	s.HealthCheckPort = in.HealthCheckPort
	s.RconHost = in.RconHost
	s.RconPort = in.RconPort
	if in.RconPassword != "" {
		s.RconPassword = in.RconPassword
	}
	if in.ConsoleMode != "" {
		s.ConsoleMode = in.ConsoleMode
	}
}
