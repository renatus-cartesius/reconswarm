package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigValidation(t *testing.T) {
	// Create a temp directory for test configs
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	// Test: empty provisioner type should fail
	t.Run("empty provisioner type", func(t *testing.T) {
		config := `
server:
  port: 50051
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: ""
  yandex_cloud:
    iam_token: "test-token"
    folder_id: "test-folder"
workers:
  max_workers: 5
`
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromFile(configPath)
		if err == nil {
			t.Error("Expected error for empty provisioner type")
		}
	})

	// Test: missing iam_token should fail
	t.Run("missing iam_token", func(t *testing.T) {
		config := `
server:
  port: 50051
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: yandex_cloud
  yandex_cloud:
    folder_id: "test-folder"
workers:
  max_workers: 5
`
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromFile(configPath)
		if err == nil {
			t.Error("Expected error for missing iam_token")
		}
	})

	// Test: missing folder_id should fail
	t.Run("missing folder_id", func(t *testing.T) {
		config := `
server:
  port: 50051
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: yandex_cloud
  yandex_cloud:
    iam_token: "test-token"
workers:
  max_workers: 5
`
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromFile(configPath)
		if err == nil {
			t.Error("Expected error for missing folder_id")
		}
	})

	// Test: valid config should pass
	t.Run("valid config", func(t *testing.T) {
		config := `
server:
  port: 50051
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: yandex_cloud
  yandex_cloud:
    iam_token: "test-token"
    folder_id: "test-folder"
workers:
  max_workers: 5
`
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromFile(configPath)
		if err != nil {
			t.Fatalf("Expected valid config to load, got error: %v", err)
		}

		if cfg.Server.Port != 50051 {
			t.Errorf("Expected port 50051, got %d", cfg.Server.Port)
		}
		if cfg.Provisioner.Type != ProviderYandexCloud {
			t.Errorf("Expected provisioner type %s, got %s", ProviderYandexCloud, cfg.Provisioner.Type)
		}
		if cfg.Provisioner.YandexCloud.IAMToken != "test-token" {
			t.Errorf("Expected iam_token 'test-token', got '%s'", cfg.Provisioner.YandexCloud.IAMToken)
		}
	})

	// Test: unsupported provider type should fail
	t.Run("unsupported provider type", func(t *testing.T) {
		config := `
server:
  port: 50051
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: aws
workers:
  max_workers: 5
`
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromFile(configPath)
		if err == nil {
			t.Error("Expected error for unsupported provider type")
		}
	})

	// Test: invalid port should fail
	t.Run("invalid port", func(t *testing.T) {
		config := `
server:
  port: 0
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: yandex_cloud
  yandex_cloud:
    iam_token: "test-token"
    folder_id: "test-folder"
workers:
  max_workers: 5
`
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromFile(configPath)
		if err == nil {
			t.Error("Expected error for invalid port")
		}
	})
}

func TestEnvVarExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	// Set test env vars
	os.Setenv("TEST_YC_TOKEN", "expanded-token")
	os.Setenv("TEST_YC_FOLDER", "expanded-folder")
	defer func() {
		os.Unsetenv("TEST_YC_TOKEN")
		os.Unsetenv("TEST_YC_FOLDER")
	}()

	config := `
server:
  port: 50051
etcd:
  endpoints:
    - "localhost:2379"
provisioner:
  type: yandex_cloud
  yandex_cloud:
    iam_token: "${TEST_YC_TOKEN}"
    folder_id: "${TEST_YC_FOLDER}"
workers:
  max_workers: 5
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Provisioner.YandexCloud.IAMToken != "expanded-token" {
		t.Errorf("Expected iam_token 'expanded-token', got '%s'", cfg.Provisioner.YandexCloud.IAMToken)
	}
	if cfg.Provisioner.YandexCloud.FolderID != "expanded-folder" {
		t.Errorf("Expected folder_id 'expanded-folder', got '%s'", cfg.Provisioner.YandexCloud.FolderID)
	}
}

func TestDefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	// Minimal config - should use defaults
	config := `
provisioner:
  type: yandex_cloud
  yandex_cloud:
    iam_token: "test-token"
    folder_id: "test-folder"
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults
	if cfg.Server.Port != 50051 {
		t.Errorf("Expected default port 50051, got %d", cfg.Server.Port)
	}
	if len(cfg.Etcd.Endpoints) != 1 || cfg.Etcd.Endpoints[0] != "localhost:2379" {
		t.Errorf("Expected default etcd endpoint 'localhost:2379', got %v", cfg.Etcd.Endpoints)
	}
	if cfg.Workers.MaxWorkers != 5 {
		t.Errorf("Expected default max_workers 5, got %d", cfg.Workers.MaxWorkers)
	}
	if cfg.Provisioner.YandexCloud.DefaultZone != "ru-central1-b" {
		t.Errorf("Expected default zone 'ru-central1-b', got '%s'", cfg.Provisioner.YandexCloud.DefaultZone)
	}
}
