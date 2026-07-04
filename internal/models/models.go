package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username           string     `gorm:"uniqueIndex;not null" json:"username"`
	Email              string     `gorm:"index" json:"email"`
	PasswordHash       string     `gorm:"not null" json:"-"`
	IsAdmin            bool       `gorm:"default:false" json:"is_admin"`
	IsRoot             bool       `gorm:"default:false" json:"is_root"` // superadmin: permanent deletes, panel settings
	Disabled           bool       `gorm:"default:false" json:"disabled"`
	MustChangePassword bool       `gorm:"default:false" json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	TokenVersion       int        `gorm:"default:0" json:"-"` // bump to revoke all sessions
	TOTPSecret         string     `json:"-"`
	TOTPEnabled        bool       `gorm:"default:false" json:"totp_enabled"`
	DiscordID          string     `gorm:"index" json:"discord_id"`
	DiscordUsername    string     `json:"discord_username"`
	Timezone           string     `gorm:"default:''" json:"timezone"`
	Servers            []Server   `gorm:"many2many:user_servers;" json:"-"`
}

func (User) TableName() string { return "users" }

type Server struct {
	gorm.Model
	Name        string `gorm:"uniqueIndex;not null" json:"name"`
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	ConfigJSON  string `json:"config_json"` // ports, volumes, env as JSON
	NodeID      uint   `gorm:"default:0;index" json:"node_id"`
	StackID     uint   `gorm:"default:0;index" json:"stack_id"`
	DependsOn   string `json:"depends_on"` // server name that must be started first
	Folder      string `gorm:"index" json:"folder"`
	Icon        string `json:"icon"`

	// Scheduling
	CronSchedule string `json:"cron_schedule"`
	CronAction   string `gorm:"default:'restart'" json:"cron_action"` // restart|stop|start|command
	CronCommand  string `json:"cron_command"`

	BackupSchedule  string `json:"backup_schedule"`
	BackupRetention int    `gorm:"default:7" json:"backup_retention"`
	BackupTarget    string `gorm:"default:'local'" json:"backup_target"` // local|s3|sftp

	// Runtime behaviour
	StopTimeout   int     `gorm:"default:10" json:"stop_timeout"` // seconds
	StopSignal    string  `json:"stop_signal"`                    // e.g. SIGINT
	RestartPolicy string  `gorm:"default:'no'" json:"restart_policy"`
	CPULimit      float64 `gorm:"default:0" json:"cpu_limit"` // cores, 0 = unlimited
	MemoryLimitMB int64   `gorm:"default:0" json:"memory_limit_mb"`
	NetworkMode   string  `json:"network_mode"`
	AutoRestart   bool    `gorm:"default:false" json:"auto_restart"` // panel-side crash restart
	AutoUpdate    bool    `gorm:"default:false" json:"auto_update"`  // notify on new image

	// Health
	HealthCheckType string `gorm:"default:'docker'" json:"health_check_type"` // none|docker|tcp
	HealthCheckPort int    `json:"health_check_port"`

	// Console / RCON
	RconHost     string `json:"rcon_host"`
	RconPort     int    `json:"rcon_port"`
	RconPassword string `json:"-"`
	ConsoleMode  string `gorm:"default:'attach'" json:"console_mode"` // attach|rcon|none

	// Logs
	PersistLogs bool `gorm:"default:false" json:"persist_logs"`

	Users []User `gorm:"many2many:user_servers;" json:"-"`
}

func (Server) TableName() string { return "servers" }

type UserServer struct {
	UserID           uint       `gorm:"primaryKey" json:"user_id"`
	ServerID         uint       `gorm:"primaryKey" json:"server_id"`
	CanStart         bool       `gorm:"default:true" json:"can_start"`
	CanStop          bool       `gorm:"default:true" json:"can_stop"`
	CanRestart       bool       `gorm:"default:true" json:"can_restart"`
	CanViewLogs      bool       `gorm:"default:true" json:"can_view_logs"`
	CanSendCommands  bool       `gorm:"default:false" json:"can_send_commands"`
	CanEditConfig    bool       `gorm:"default:false" json:"can_edit_config"`
	CanAccessFiles   bool       `gorm:"default:false" json:"can_access_files"`
	CanManageBackups bool       `gorm:"default:false" json:"can_manage_backups"`
	ExpiresAt        *time.Time `json:"expires_at"` // temporary access
}

func (UserServer) TableName() string { return "user_servers" }

// Perms is the flattened effective permission set for a user on a server.
type Perms struct {
	CanStart         bool `json:"can_start"`
	CanStop          bool `json:"can_stop"`
	CanRestart       bool `json:"can_restart"`
	CanViewLogs      bool `json:"can_view_logs"`
	CanSendCommands  bool `json:"can_send_commands"`
	CanEditConfig    bool `json:"can_edit_config"`
	CanAccessFiles   bool `json:"can_access_files"`
	CanManageBackups bool `json:"can_manage_backups"`
}

func AllPerms() Perms {
	return Perms{true, true, true, true, true, true, true, true}
}

func (p Perms) Merge(o Perms) Perms {
	return Perms{
		p.CanStart || o.CanStart,
		p.CanStop || o.CanStop,
		p.CanRestart || o.CanRestart,
		p.CanViewLogs || o.CanViewLogs,
		p.CanSendCommands || o.CanSendCommands,
		p.CanEditConfig || o.CanEditConfig,
		p.CanAccessFiles || o.CanAccessFiles,
		p.CanManageBackups || o.CanManageBackups,
	}
}

func (us UserServer) Perms() Perms {
	return Perms{us.CanStart, us.CanStop, us.CanRestart, us.CanViewLogs,
		us.CanSendCommands, us.CanEditConfig, us.CanAccessFiles, us.CanManageBackups}
}

// Groups: assign many users to many servers in one step.
type Group struct {
	gorm.Model
	Name string `gorm:"uniqueIndex;not null" json:"name"`
}

func (Group) TableName() string { return "groups" }

type UserGroup struct {
	UserID  uint `gorm:"primaryKey" json:"user_id"`
	GroupID uint `gorm:"primaryKey" json:"group_id"`
}

func (UserGroup) TableName() string { return "user_groups" }

type GroupServer struct {
	GroupID          uint `gorm:"primaryKey" json:"group_id"`
	ServerID         uint `gorm:"primaryKey" json:"server_id"`
	CanStart         bool `gorm:"default:true" json:"can_start"`
	CanStop          bool `gorm:"default:true" json:"can_stop"`
	CanRestart       bool `gorm:"default:true" json:"can_restart"`
	CanViewLogs      bool `gorm:"default:true" json:"can_view_logs"`
	CanSendCommands  bool `gorm:"default:false" json:"can_send_commands"`
	CanEditConfig    bool `gorm:"default:false" json:"can_edit_config"`
	CanAccessFiles   bool `gorm:"default:false" json:"can_access_files"`
	CanManageBackups bool `gorm:"default:false" json:"can_manage_backups"`
}

func (GroupServer) TableName() string { return "group_servers" }

func (gs GroupServer) Perms() Perms {
	return Perms{gs.CanStart, gs.CanStop, gs.CanRestart, gs.CanViewLogs,
		gs.CanSendCommands, gs.CanEditConfig, gs.CanAccessFiles, gs.CanManageBackups}
}

// Session is a refresh-token session (device login).
type Session struct {
	gorm.Model
	UserID           uint      `gorm:"index" json:"user_id"`
	RefreshTokenHash string    `gorm:"index" json:"-"`
	UserAgent        string    `json:"user_agent"`
	IP               string    `json:"ip"`
	ExpiresAt        time.Time `json:"expires_at"`
	LastUsedAt       time.Time `json:"last_used_at"`
	Revoked          bool      `gorm:"default:false" json:"revoked"`
}

func (Session) TableName() string { return "sessions" }

// Invitation is a one-time signup link created by an admin.
type Invitation struct {
	gorm.Model
	Token     string     `gorm:"uniqueIndex" json:"token"`
	IsAdmin   bool       `json:"is_admin"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedByID  uint       `json:"used_by_id"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedBy string     `json:"created_by"`
}

func (Invitation) TableName() string { return "invitations" }

type PasswordReset struct {
	gorm.Model
	UserID    uint      `gorm:"index" json:"user_id"`
	TokenHash string    `gorm:"index" json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

func (PasswordReset) TableName() string { return "password_resets" }

type AuditLog struct {
	gorm.Model
	UserID     uint   `gorm:"index" json:"user_id"`
	Username   string `json:"username"`
	Action     string `gorm:"index" json:"action"`
	ServerID   uint   `gorm:"index" json:"server_id"`
	ServerName string `json:"server_name"`
	Details    string `json:"details"`
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	Success    bool   `gorm:"default:true" json:"success"`
	RequestID  string `json:"request_id"`
	DiffJSON   string `json:"diff_json"` // before/after for updates
}

func (AuditLog) TableName() string { return "audit_logs" }

// Node is a Docker host (local socket or remote TCP/TLS).
type Node struct {
	gorm.Model
	Name    string `gorm:"uniqueIndex;not null" json:"name"`
	Address string `json:"address"` // e.g. unix:///var/run/docker.sock or tcp://10.0.0.5:2376
	TLSCA   string `json:"-"`
	TLSCert string `json:"-"`
	TLSKey  string `json:"-"`
	Enabled bool   `gorm:"default:true" json:"enabled"`
}

func (Node) TableName() string { return "nodes" }

// Stack groups multiple servers into one multi-container deployment.
type Stack struct {
	gorm.Model
	Name    string `gorm:"uniqueIndex;not null" json:"name"`
	Network string `json:"network"` // dedicated docker network for the stack
}

func (Stack) TableName() string { return "stacks" }

type Backup struct {
	gorm.Model
	ServerID   uint   `gorm:"index" json:"server_id"`
	ServerName string `json:"server_name"`
	FileName   string `json:"file_name"`
	SizeBytes  int64  `json:"size_bytes"`
	Status     string `gorm:"default:'pending'" json:"status"` // pending|running|done|failed
	Target     string `gorm:"default:'local'" json:"target"`
	Note       string `json:"note"`
	Error      string `json:"error"`
}

func (Backup) TableName() string { return "backups" }

type MetricSample struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ServerID   uint      `gorm:"index:idx_metric_server_at" json:"server_id"`
	At         time.Time `gorm:"index:idx_metric_server_at" json:"at"`
	CPUPercent float64   `json:"cpu_percent"`
	MemUsed    int64     `json:"mem_used"`
	MemLimit   int64     `json:"mem_limit"`
	NetRx      int64     `json:"net_rx"`
	NetTx      int64     `json:"net_tx"`
	Running    bool      `json:"running"`
}

func (MetricSample) TableName() string { return "metric_samples" }

// Notification is an in-app notification (bell icon).
type Notification struct {
	gorm.Model
	UserID   uint       `gorm:"index" json:"user_id"`
	ServerID uint       `json:"server_id"`
	Level    string     `gorm:"default:'info'" json:"level"` // info|warn|error|success
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	ReadAt   *time.Time `json:"read_at"`
}

func (Notification) TableName() string { return "notifications" }

// NotificationPref: which events a user wants, per server (ServerID 0 = all).
type NotificationPref struct {
	UserID   uint `gorm:"primaryKey" json:"user_id"`
	ServerID uint `gorm:"primaryKey" json:"server_id"`
	OnDown   bool `gorm:"default:true" json:"on_down"`
	OnUp     bool `gorm:"default:false" json:"on_up"`
	OnCrash  bool `gorm:"default:true" json:"on_crash"`
	OnBackup bool `gorm:"default:false" json:"on_backup"`
	OnLogin  bool `gorm:"default:false" json:"on_login"`
}

func (NotificationPref) TableName() string { return "notification_prefs" }

type PushSubscription struct {
	gorm.Model
	UserID   uint   `gorm:"index" json:"user_id"`
	Endpoint string `gorm:"uniqueIndex:idx_push_endpoint,length:255" json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

func (PushSubscription) TableName() string { return "push_subscriptions" }

// Setting is a key/value panel setting (maintenance banner, webhooks, vapid keys, ...).
type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

func (Setting) TableName() string { return "settings" }

// CommandSnippet is a saved console command/macro for a server.
type CommandSnippet struct {
	gorm.Model
	ServerID uint   `gorm:"index" json:"server_id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
}

func (CommandSnippet) TableName() string { return "command_snippets" }

// LogAlert notifies when a regex matches a server's log stream.
type LogAlert struct {
	gorm.Model
	ServerID uint   `gorm:"index" json:"server_id"`
	Pattern  string `json:"pattern"`
	Enabled  bool   `gorm:"default:true" json:"enabled"`
}

func (LogAlert) TableName() string { return "log_alerts" }

// All returns every model for AutoMigrate.
func All() []interface{} {
	return []interface{}{
		&User{}, &Server{}, &UserServer{}, &Group{}, &UserGroup{}, &GroupServer{},
		&Session{}, &Invitation{}, &PasswordReset{}, &AuditLog{}, &Node{}, &Stack{},
		&Backup{}, &MetricSample{}, &Notification{}, &NotificationPref{},
		&PushSubscription{}, &Setting{}, &CommandSnippet{}, &LogAlert{},
	}
}
