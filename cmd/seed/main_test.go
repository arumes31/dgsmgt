package main

import (
	"os"
	"testing"
	"gorm.io/gorm"
)

func TestRun(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	defer os.Unsetenv("DATABASE_URL")
	if err := Run(); err != nil { t.Fatalf("Run failed: %v", err) }
}

func TestRunFailures(t *testing.T) {
	t.Run("DBFail", func(t *testing.T) {
		os.Setenv("DATABASE_URL", "/invalid/path")
		defer os.Unsetenv("DATABASE_URL")
		if err := Run(); err == nil { t.Error("expected error") }
	})

	t.Run("MigrateFail", func(t *testing.T) {
		f, _ := os.CreateTemp("", "readonly*.db")
		f.Close()
		_ = os.Chmod(f.Name(), 0444)
		defer os.Remove(f.Name())
		os.Setenv("DATABASE_URL", f.Name())
		defer os.Unsetenv("DATABASE_URL")
		if err := Run(); err == nil { t.Error("expected error") }
	})
}

func TestMainFunction(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	defer os.Unsetenv("DATABASE_URL")
	main()
}

func TestMainFunctionFail(t *testing.T) {
	os.Setenv("DATABASE_URL", "/invalid/path")
	defer os.Unsetenv("DATABASE_URL")
	
	exited := false
	oldExit := osExit
	defer func() { osExit = oldExit }()
	
	osExit = func(code int) {
		exited = true
	}
	
	main()
	if !exited {
		t.Error("expected main to exit on failure")
	}
}

func TestRunEmptyDSN(t *testing.T) {
	os.Setenv("DATABASE_URL", "")
	defer os.Unsetenv("DATABASE_URL")
	// If it falls back to dgsmgt.db, it will succeed to init DB or fail depending on permissions.
	// We just want to cover the `dsn == ""` branch.
	_ = Run()
}

func TestAssignAccessFail(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	defer os.Unsetenv("DATABASE_URL")
	err := Run()
	if err != nil { t.Fatalf("setup failed") }

	// Now register a callback to fail updates/creates
	forcedErr := os.ErrPermission
	database.Callback().Create().Before("gorm:create").Register("force_fail_create", func(db *gorm.DB) { db.Error = forcedErr })
	database.Callback().Update().Before("gorm:update").Register("force_fail_update", func(db *gorm.DB) { db.Error = forcedErr })

	// run assignAccess again, it will fail
	assignAccess()
}
