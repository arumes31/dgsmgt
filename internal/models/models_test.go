package models

import (
	"testing"
	"gorm.io/gorm"
)

func TestModels(t *testing.T) {
	u := User{
		Model: gorm.Model{ID: 1},
		Username: "admin",
		PasswordHash: "secret",
		IsAdmin: true,
	}
	if u.Username != "admin" {
		t.Errorf("Expected username admin, got %s", u.Username)
	}
	if u.TableName() != "users" {
		t.Errorf("Expected table name users, got %s", u.TableName())
	}

	s := Server{
		Model: gorm.Model{ID: 1},
		Name: "testserver",
		ContainerID: "cid",
		Image: "image",
		ConfigJSON: "{}",
		CronSchedule: "* * * * *",
	}
	if s.Name != "testserver" {
		t.Errorf("Expected name testserver, got %s", s.Name)
	}
	if s.TableName() != "servers" {
		t.Errorf("Expected table name servers, got %s", s.TableName())
	}

	us := UserServer{
		UserID: 1,
		ServerID: 1,
		CanStart: true,
		CanStop: true,
		CanRestart: true,
		CanViewLogs: true,
	}
	if us.UserID != 1 {
		t.Errorf("Expected UserID 1, got %d", us.UserID)
	}
	if us.TableName() != "user_servers" {
		t.Errorf("Expected table name user_servers, got %s", us.TableName())
	}

	al := AuditLog{
		Model: gorm.Model{ID: 1},
		UserID: 1,
		Username: "admin",
		Action: "start",
		ServerID: 1,
		ServerName: "testserver",
		Details: "server started",
	}
	if al.Action != "start" {
		t.Errorf("Expected action start, got %s", al.Action)
	}
	if al.TableName() != "audit_logs" {
		t.Errorf("Expected table name audit_logs, got %s", al.TableName())
	}
}
