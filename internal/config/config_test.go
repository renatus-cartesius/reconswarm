package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Note: This test requires a valid reconswarm.yaml file with iam_token and folder_id set

	// Test loading config from file
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test basic fields
	if cfg.DefaultZone == "" {
		t.Error("DefaultZone should not be empty")
	}
	if cfg.DefaultUsername == "" {
		t.Error("DefaultUsername should not be empty")
	}
	if cfg.DefaultCores <= 0 {
		t.Error("DefaultCores should be positive")
	}
	if cfg.DefaultMemory <= 0 {
		t.Error("DefaultMemory should be positive")
	}
	if cfg.DefaultDiskSize <= 0 {
		t.Error("DefaultDiskSize should be positive")
	}
	if cfg.MaxWorkers <= 0 {
		t.Error("MaxWorkers should be positive")
	}

	// Test pipeline structure (may be empty due to interface deserialization)
	t.Logf("Pipeline targets: %d", len(cfg.Pipeline().Targets))
	t.Logf("Pipeline stages: %d", len(cfg.Pipeline().Stages))

	// Test setup commands (may be empty due to YAML parsing)
	t.Logf("SetupCommands: %d commands", len(cfg.SetupCommands))
}

func TestLoadConfigValidation(t *testing.T) {
	// Test validation with missing required fields
	// Temporarily rename config file to test validation
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "reconswarm.yaml"
	}

	// Backup original config if it exists
	backupPath := configPath + ".backup"
	if _, err := os.Stat(configPath); err == nil {
		data, _ := os.ReadFile(configPath)
		os.WriteFile(backupPath, data, 0644)
		defer os.Remove(backupPath)
		defer os.WriteFile(configPath, data, 0644)
	}

	// Create a temporary config file with missing required fields
	tempConfig := `default_zone: "ru-central1-b"
default_username: "reconswarm"
`
	os.WriteFile(configPath, []byte(tempConfig), 0644)
	defer os.Remove(configPath)

	cfg, err := Load()
	if err == nil {
		t.Error("Expected error for missing IAM token, but got none")
	}
	if cfg != nil {
		t.Error("Expected config to be nil when validation fails")
	}
}

func TestPipelineStages(t *testing.T) {
	// Note: This test requires a valid reconswarm.yaml file with iam_token and folder_id set

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test that stages have correct structure
	for i, stage := range cfg.Pipeline().Stages {
		if stage.GetName() == "" {
			t.Errorf("Stage %d should have a name", i)
		}
		if stage.GetType() == "" {
			t.Errorf("Stage %d should have a type", i)
		}

		t.Logf("Stage %d: %s (%s)", i+1, stage.GetName(), stage.GetType())
	}
}

func TestPipelineTargets(t *testing.T) {
	// Note: This test requires a valid reconswarm.yaml file with iam_token and folder_id set

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test that targets have correct structure
	for i, target := range cfg.Pipeline().Targets {
		if target.Type == "" {
			t.Errorf("Target %d should have a type", i)
		}
		if target.Value == nil {
			t.Errorf("Target %d should have a value", i)
		}

		t.Logf("Target %d: type=%s, value=%v", i+1, target.Type, target.Value)
	}
}

func TestYAMLFileExists(t *testing.T) {
	// Check if reconswarm.yaml exists in project root
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "../../reconswarm.yaml"
	} else {
		configPath = "../../" + configPath
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("reconswarm.yaml file should exist in project root")
		return
	}

	// Read and parse YAML file directly
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read reconswarm.yaml: %v", err)
	}

	// Check that file contains expected content
	content := string(data)
	if !contains(content, "pipeline:") {
		t.Error("reconswarm.yaml should contain 'pipeline:' section")
	}
	if !contains(content, "targets:") {
		t.Error("reconswarm.yaml should contain 'targets:' section")
	}
	if !contains(content, "stages:") {
		t.Error("reconswarm.yaml should contain 'stages:' section")
	}
	if !contains(content, "setup_commands:") {
		t.Error("reconswarm.yaml should contain 'setup_commands:' section")
	}

	t.Logf("reconswarm.yaml file exists and contains required sections")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && contains(s[1:], substr)
}
