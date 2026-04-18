package db

import (
	"dgsmgt/internal/models"
	"os"
	"testing"
)

func TestInitDB(t *testing.T) {
	dsn := "test_dgsmgt.db"
	defer os.Remove(dsn)

	db, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	if db == nil {
		t.Fatal("Expected db instance, got nil")
	}

	if DB == nil {
		t.Fatal("Expected global DB to be set, got nil")
	}

	// Verify tables were created
	if !db.Migrator().HasTable(&models.User{}) {
		t.Error("Table users was not created")
	}
}

func TestInitDBError(t *testing.T) {
	// Use an invalid DSN or a path that's not writable
	_, err := InitDB("/nonexistent_dir_123/test.db")
	if err == nil {
		t.Error("Expected error for invalid DSN, got nil")
	}
}

func TestInitDBMigrateError(t *testing.T) {
	// Create an empty db file
	dbFile := "ro.db"
	_ = os.WriteFile(dbFile, []byte(""), 0444)
	defer os.Remove(dbFile)
	
	// Open it with a query string that makes it read-only for SQLite
	// This will allow gorm.Open to succeed but AutoMigrate to fail
	_, err := InitDB("file:" + dbFile + "?mode=ro")
	if err == nil {
		t.Error("Expected AutoMigrate to fail on read-only DB")
	}
}
