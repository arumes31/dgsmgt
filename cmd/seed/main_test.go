package main

import (
	"os"
	"os/exec"
	"testing"
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
	if os.Getenv("BE_MAIN") == "1" {
		os.Setenv("DATABASE_URL", ":memory:")
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainFunction")
	cmd.Env = append(os.Environ(), "BE_MAIN=1")
	_ = cmd.Run()
}

func TestMainFunctionFail(t *testing.T) {
	if os.Getenv("BE_MAIN_FAIL") == "1" {
		os.Setenv("DATABASE_URL", "/invalid/path")
		osExit = func(code int) { panic(code) }
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainFunctionFail")
	cmd.Env = append(os.Environ(), "BE_MAIN_FAIL=1")
	if err := cmd.Run(); err == nil { t.Error("expected error") }
}
