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
	var originalData []byte
	if _, err := os.Stat(configPath); err == nil {
		var err error
		originalData, err = os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read original config file: %v", err)
		}

		if err := os.WriteFile(backupPath, originalData, 0644); err != nil {
			t.Fatalf("failed to create backup config file: %v", err)
		}
		defer func() {
			if err := os.Remove(backupPath); err != nil {
				t.Errorf("failed to remove backup file: %v", err)
			}
		}()

		defer func() {
			if err := os.WriteFile(configPath, originalData, 0644); err != nil {
				t.Errorf("failed to restore original config file: %v", err)
			}
		}()
	}

	// Create a temporary config file with missing required fields
	tempConfig := `default_zone: "ru-central1-b"
default_username: "reconswarm"
`
	if err := os.WriteFile(configPath, []byte(tempConfig), 0644); err != nil {
		t.Fatalf("failed to write temporary config file: %v", err)
	}
	defer func() {
		if err := os.Remove(configPath); err != nil {
			t.Errorf("failed to remove temporary config file: %v", err)
		}
	}()

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
