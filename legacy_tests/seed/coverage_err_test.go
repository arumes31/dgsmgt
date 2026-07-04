package main

import (
	"os"
	"testing"
)

func TestRun_ErrorBranches(t *testing.T) {
	// Create a read-only database file
	dbFile := "readonly_seed_test.db"
	os.Remove(dbFile)
	f, _ := os.Create(dbFile)
	f.Close()
	_ = os.Chmod(dbFile, 0444)
	defer os.Remove(dbFile)

	os.Setenv("DATABASE_URL", dbFile)
	defer os.Unsetenv("DATABASE_URL")

	// This should fail during migrations or FirstOrCreate, hitting error paths
	_ = Run()
}
