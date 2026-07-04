package db

import (
	"dgsmgt/internal/models"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Tests swap the Open hook so migrations run against in-memory SQLite; the
// runtime driver is Postgres-only.

func TestInitDB(t *testing.T) {
	orig := Open
	Open = func(dsn string) (gorm.Dialector, error) { return sqlite.Open(":memory:"), nil }
	defer func() { Open = orig }()

	db, err := InitDB("ignored")
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	if db == nil {
		t.Fatal("Expected db instance, got nil")
	}
	if DB == nil {
		t.Fatal("Expected global DB to be set, got nil")
	}
	if !db.Migrator().HasTable(&models.User{}) {
		t.Error("Table users was not created")
	}
	if !db.Migrator().HasTable(&models.Session{}) {
		t.Error("Table sessions was not created")
	}
	if !db.Migrator().HasTable(&models.Backup{}) {
		t.Error("Table backups was not created")
	}
}

func TestInitDBOpenError(t *testing.T) {
	orig := Open
	Open = func(dsn string) (gorm.Dialector, error) { return nil, errors.New("bad dsn") }
	defer func() { Open = orig }()

	if _, err := InitDB("whatever"); err == nil {
		t.Error("Expected error for failing Open hook, got nil")
	}
}

func TestInitDBConnectError(t *testing.T) {
	// Real Postgres dialector against a closed port must fail.
	if _, err := InitDB("host=127.0.0.1 port=1 user=x dbname=x connect_timeout=1"); err == nil {
		t.Error("Expected connection error, got nil")
	}
}
