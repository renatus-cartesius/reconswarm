package config

import (
	"os"
	"testing"
)

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

// TestPipelineStages is skipped as it requires a valid reconswarm.yaml file
// which should not be committed to the repository

// TestPipelineTargets is skipped as it requires a valid reconswarm.yaml file
// which should not be committed to the repository

// TestYAMLFileExists is skipped as it requires a valid reconswarm.yaml file
// which should not be committed to the repository
