package config

import (
	"fmt"
	"os"

	"reconswarm/internal/pipeline"

	"gopkg.in/yaml.v2"
)

// Config contains application configuration
type Config struct {
	// Yandex Cloud connection parameters
	IAMToken string `yaml:"iam_token"`
	FolderID string `yaml:"folder_id"`

	// Default settings
	DefaultZone     string `yaml:"default_zone"`
	DefaultImage    string `yaml:"default_image"`
	DefaultUsername string `yaml:"default_username"`

	// Default VM parameters
	DefaultCores    int   `yaml:"default_cores"`
	DefaultMemory   int64 `yaml:"default_memory"`    // in GB
	DefaultDiskSize int64 `yaml:"default_disk_size"` // in GB

	// Setup commands for VM configuration
	SetupCommands []string `yaml:"setup_commands"`

	// Pipeline configuration (raw for custom deserialization)
	PipelineRaw pipeline.PipelineRaw `yaml:"pipeline"`

	// Max number of workers
	MaxWorkers int `yaml:"max_workers"`
}

// Pipeline returns the parsed pipeline configuration
func (c *Config) Pipeline() pipeline.Pipeline {
	return c.PipelineRaw.ToPipeline()
}

// Load loads configuration from YAML file
func Load() (*Config, error) {
	config := &Config{
		DefaultZone:     "ru-central1-b",
		DefaultImage:    "", // will use default image
		DefaultUsername: "reconswarm",
		DefaultCores:    2,
		DefaultMemory:   2,  // 2GB
		DefaultDiskSize: 20, // 20GB
		MaxWorkers:      5,
	}

	// Try to load from YAML file first
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "reconswarm.yaml"
	}

	if _, err := os.Stat(configPath); err == nil {
		// Load from YAML file
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %v", err)
		}

		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %v", err)
		}
	}

	// Expand environment variables in string fields
	config.IAMToken = os.ExpandEnv(config.IAMToken)
	config.FolderID = os.ExpandEnv(config.FolderID)
	config.DefaultZone = os.ExpandEnv(config.DefaultZone)
	config.DefaultImage = os.ExpandEnv(config.DefaultImage)
	config.DefaultUsername = os.ExpandEnv(config.DefaultUsername)

	// Expand environment variables in setup commands
	for i, cmd := range config.SetupCommands {
		config.SetupCommands[i] = os.ExpandEnv(cmd)
	}

	// Override with environment variables if set (from secrets-setup.sh)
	if token := os.Getenv("YC_TOKEN"); token != "" {
		config.IAMToken = token
	}

	if folderID := os.Getenv("YC_FOLDER_ID"); folderID != "" {
		config.FolderID = folderID
	}

	// Validate required parameters
	if config.IAMToken == "" {
		return nil, fmt.Errorf("IAM token is required (set iam_token in config file or YC_TOKEN environment variable)")
	}

	if config.FolderID == "" {
		return nil, fmt.Errorf("Folder ID is required (set folder_id in config file or YC_FOLDER_ID environment variable)")
	}

	return config, nil
}
