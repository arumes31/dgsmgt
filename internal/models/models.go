package models

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	IsAdmin      bool   `gorm:"default:false"`
	Servers      []Server `gorm:"many2many:user_servers;"`
}

type Server struct {
	gorm.Model
	Name         string `gorm:"uniqueIndex;not null"`
	ContainerID  string
	Image        string
	ConfigJSON   string // Stores ports, volumes, env vars as JSON
	CronSchedule string // For scheduled restarts
	Users        []User `gorm:"many2many:user_servers;"`
}

type UserServer struct {
	UserID      uint `gorm:"primaryKey"`
	ServerID    uint `gorm:"primaryKey"`
	CanStart    bool `gorm:"default:true"`
	CanStop     bool `gorm:"default:true"`
	CanRestart  bool `gorm:"default:true"`
	CanViewLogs bool `gorm:"default:true"`
}

type AuditLog struct {
	gorm.Model
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	ServerID  uint   `json:"server_id"`
	ServerName string `json:"server_name"`
	Details   string `json:"details"`
}
