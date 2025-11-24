package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	content := `[{"instanceType": "t2.micro", "maxRuntimeHours": 24}]`
	tmpfile, err := os.CreateTemp("", "config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Set environment variables
	os.Setenv("CONFIG_PATH", tmpfile.Name())
	os.Setenv("AWS_REGION", "us-east-1")
	os.Setenv("SCHEDULE", "* * * * *")
	os.Setenv("DRY_RUN", "true")
	defer os.Unsetenv("CONFIG_PATH")
	defer os.Unsetenv("AWS_REGION")
	defer os.Unsetenv("SCHEDULE")
	defer os.Unsetenv("DRY_RUN")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Targets) != 1 {
		t.Errorf("Expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].InstanceType != "t2.micro" {
		t.Errorf("Expected instance type t2.micro, got %s", cfg.Targets[0].InstanceType)
	}
	if cfg.AWSRegion != "us-east-1" {
		t.Errorf("Expected region us-east-1, got %s", cfg.AWSRegion)
	}
	if !cfg.DryRun {
		t.Error("Expected DryRun to be true")
	}
}

func TestLoadMissingConfigPath(t *testing.T) {
	os.Unsetenv("CONFIG_PATH")
	_, err := Load()
	if err == nil {
		t.Error("Expected error when CONFIG_PATH is missing, got nil")
	}
}
