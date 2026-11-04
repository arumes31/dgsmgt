package models

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null" json:"-"`
	IsAdmin      bool   `gorm:"default:false"`
	// MustChangePassword blocks API access (except changing the password) until the user sets a new password
	MustChangePassword bool `gorm:"default:false"`
	// TokenVersion is embedded in issued JWTs; bumping it revokes all existing tokens of the user
	TokenVersion uint     `gorm:"default:0" json:"-"`
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
	// Permissions are deny-by-default so an explicit false is never replaced
	// by an insertion default.
	CanStart    bool `gorm:"default:false"`
	CanStop     bool `gorm:"default:false"`
	CanRestart  bool `gorm:"default:false"`
	CanViewLogs bool `gorm:"default:false"`
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
