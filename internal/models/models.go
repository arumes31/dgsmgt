package models

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username     string   `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string   `gorm:"not null" json:"-"`
	IsAdmin      bool     `gorm:"default:false" json:"is_admin"`
	Servers      []Server `gorm:"many2many:user_servers;" json:"-"`
}

func (User) TableName() string {
	return "users"
}

type Server struct {
	gorm.Model
	Name         string `gorm:"uniqueIndex;not null" json:"name"`
	ContainerID  string `json:"container_id"`
	Image        string `json:"image"`
	ConfigJSON   string `json:"config_json"`    // Stores ports, volumes, env vars as JSON
	CronSchedule string `json:"cron_schedule"` // For scheduled restarts
	Users        []User `gorm:"many2many:user_servers;" json:"-"`
}

func (Server) TableName() string {
	return "servers"
}

type UserServer struct {
	UserID      uint `gorm:"primaryKey" json:"user_id"`
	ServerID    uint `gorm:"primaryKey" json:"server_id"`
	CanStart    bool `gorm:"default:true" json:"can_start"`
	CanStop     bool `gorm:"default:true" json:"can_stop"`
	CanRestart  bool `gorm:"default:true" json:"can_restart"`
	CanViewLogs bool `gorm:"default:true" json:"can_view_logs"`
}

func (UserServer) TableName() string {
	return "user_servers"
}

type AuditLog struct {
	gorm.Model
	UserID     uint   `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	ServerID   uint   `json:"server_id"`
	ServerName string `json:"server_name"`
	Details    string `json:"details"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
