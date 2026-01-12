package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config represents the root application configuration.
// All server settings MUST be configured via config file.
// Note: Pipeline is NOT part of server config - it's passed via 'run' subcommand.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Etcd        EtcdConfig        `yaml:"etcd"`
	Provisioner ProvisionerConfig `yaml:"provisioner"`
	Workers     WorkersConfig     `yaml:"workers"`
}

// ServerConfig contains gRPC server settings
type ServerConfig struct {
	Port int `yaml:"port"`
}

// EtcdConfig contains etcd connection settings
type EtcdConfig struct {
	Endpoints   []string `yaml:"endpoints"`
	DialTimeout int      `yaml:"dial_timeout"` // seconds
	Username    string   `yaml:"username"`
	Password    string   `yaml:"password"`
}

// WorkersConfig contains worker pool settings
type WorkersConfig struct {
	MaxWorkers    int      `yaml:"max_workers"`
	SetupCommands []string `yaml:"setup_commands"`
}

// ProvisionerConfig uses discriminated union pattern.
// Type field determines which provider config is active.
type ProvisionerConfig struct {
	Type         ProviderType        `yaml:"type"`
	YandexCloud  *YandexCloudConfig  `yaml:"yandex_cloud,omitempty"`
	GCP          *GCPConfig          `yaml:"gcp,omitempty"`
	AWS          *AWSConfig          `yaml:"aws,omitempty"`
	DigitalOcean *DigitalOceanConfig `yaml:"digitalocean,omitempty"`
}

// ProviderType represents supported cloud providers
type ProviderType string

const (
	ProviderYandexCloud  ProviderType = "yandex_cloud"
	ProviderGCP          ProviderType = "gcp"
	ProviderAWS          ProviderType = "aws"
	ProviderDigitalOcean ProviderType = "digitalocean"
)

// YandexCloudConfig contains Yandex Cloud specific settings
type YandexCloudConfig struct {
	// Authentication
	IAMToken string `yaml:"iam_token"`
	FolderID string `yaml:"folder_id"`

	// VM defaults
	DefaultZone     string `yaml:"default_zone"`
	DefaultImage    string `yaml:"default_image"`
	DefaultUsername string `yaml:"default_username"`
	DefaultCores    int    `yaml:"default_cores"`
	DefaultMemory   int64  `yaml:"default_memory"`    // GB
	DefaultDiskSize int64  `yaml:"default_disk_size"` // GB
}

// GCPConfig contains Google Cloud specific settings
type GCPConfig struct {
	ProjectID       string `yaml:"project_id"`
	CredentialsPath string `yaml:"credentials_path"`

	// VM defaults
	DefaultZone     string `yaml:"default_zone"`
	DefaultImage    string `yaml:"default_image"`
	DefaultUsername string `yaml:"default_username"`
	DefaultCores    int    `yaml:"default_cores"`
	DefaultMemory   int64  `yaml:"default_memory"`    // GB
	DefaultDiskSize int64  `yaml:"default_disk_size"` // GB
}

// AWSConfig contains AWS specific settings
type AWSConfig struct {
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`

	// VM defaults
	DefaultZone     string `yaml:"default_zone"`
	DefaultImage    string `yaml:"default_image"`
	DefaultUsername string `yaml:"default_username"`
	DefaultCores    int    `yaml:"default_cores"`
	DefaultMemory   int64  `yaml:"default_memory"`    // GB
	DefaultDiskSize int64  `yaml:"default_disk_size"` // GB
}

// DigitalOceanConfig contains DigitalOcean specific settings
type DigitalOceanConfig struct {
	Token string `yaml:"token"`

	// VM defaults
	DefaultRegion   string `yaml:"default_region"`
	DefaultImage    string `yaml:"default_image"`
	DefaultUsername string `yaml:"default_username"`
	DefaultCores    int    `yaml:"default_cores"`
	DefaultMemory   int64  `yaml:"default_memory"`    // GB
	DefaultDiskSize int64  `yaml:"default_disk_size"` // GB
}

// Validate validates the provisioner configuration
func (p *ProvisionerConfig) Validate() error {
	switch p.Type {
	case ProviderYandexCloud:
		if p.YandexCloud == nil {
			return fmt.Errorf("yandex_cloud config required when type is '%s'", ProviderYandexCloud)
		}
		return p.YandexCloud.Validate()
	case ProviderGCP:
		if p.GCP == nil {
			return fmt.Errorf("gcp config required when type is '%s'", ProviderGCP)
		}
		return p.GCP.Validate()
	case ProviderAWS:
		if p.AWS == nil {
			return fmt.Errorf("aws config required when type is '%s'", ProviderAWS)
		}
		return p.AWS.Validate()
	case ProviderDigitalOcean:
		if p.DigitalOcean == nil {
			return fmt.Errorf("digitalocean config required when type is '%s'", ProviderDigitalOcean)
		}
		return p.DigitalOcean.Validate()
	case "":
		return fmt.Errorf("provisioner type is required")
	default:
		return fmt.Errorf("unsupported provisioner type: %s", p.Type)
	}
}

// Validate validates Yandex Cloud configuration
func (c *YandexCloudConfig) Validate() error {
	if c.IAMToken == "" {
		return fmt.Errorf("yandex_cloud.iam_token is required")
	}
	if c.FolderID == "" {
		return fmt.Errorf("yandex_cloud.folder_id is required")
	}
	return nil
}

// Validate validates GCP configuration
func (c *GCPConfig) Validate() error {
	if c.ProjectID == "" {
		return fmt.Errorf("gcp.project_id is required")
	}
	return nil
}

// Validate validates AWS configuration
func (c *AWSConfig) Validate() error {
	if c.Region == "" {
		return fmt.Errorf("aws.region is required")
	}
	return nil
}

// Validate validates DigitalOcean configuration
func (c *DigitalOceanConfig) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("digitalocean.token is required")
	}
	return nil
}

// Load loads configuration from YAML file
func Load() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "reconswarm.yaml"
	}
	return LoadFromFile(configPath)
}

// LoadFromFile loads configuration from specified file path
func LoadFromFile(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand environment variables
	cfg.expandEnvVars()

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// defaultConfig returns configuration with sensible defaults
func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 50051,
		},
		Etcd: EtcdConfig{
			Endpoints:   []string{"localhost:2379"},
			DialTimeout: 5,
		},
		Workers: WorkersConfig{
			MaxWorkers: 5,
		},
		Provisioner: ProvisionerConfig{
			Type: ProviderYandexCloud,
			YandexCloud: &YandexCloudConfig{
				DefaultZone:     "ru-central1-b",
				DefaultUsername: "reconswarm",
				DefaultCores:    2,
				DefaultMemory:   2,
				DefaultDiskSize: 20,
			},
		},
	}
}

// expandEnvVars expands environment variables in string fields
func (c *Config) expandEnvVars() {
	// Etcd credentials
	c.Etcd.Username = os.ExpandEnv(c.Etcd.Username)
	c.Etcd.Password = os.ExpandEnv(c.Etcd.Password)
	for i, ep := range c.Etcd.Endpoints {
		c.Etcd.Endpoints[i] = os.ExpandEnv(ep)
	}

	// Provisioner settings
	if c.Provisioner.YandexCloud != nil {
		yc := c.Provisioner.YandexCloud
		yc.IAMToken = os.ExpandEnv(yc.IAMToken)
		yc.FolderID = os.ExpandEnv(yc.FolderID)
		yc.DefaultZone = os.ExpandEnv(yc.DefaultZone)
		yc.DefaultImage = os.ExpandEnv(yc.DefaultImage)
		yc.DefaultUsername = os.ExpandEnv(yc.DefaultUsername)
	}
	if c.Provisioner.GCP != nil {
		gcp := c.Provisioner.GCP
		gcp.ProjectID = os.ExpandEnv(gcp.ProjectID)
		gcp.CredentialsPath = os.ExpandEnv(gcp.CredentialsPath)
		gcp.DefaultZone = os.ExpandEnv(gcp.DefaultZone)
		gcp.DefaultImage = os.ExpandEnv(gcp.DefaultImage)
		gcp.DefaultUsername = os.ExpandEnv(gcp.DefaultUsername)
	}
	if c.Provisioner.AWS != nil {
		aws := c.Provisioner.AWS
		aws.Region = os.ExpandEnv(aws.Region)
		aws.AccessKeyID = os.ExpandEnv(aws.AccessKeyID)
		aws.SecretAccessKey = os.ExpandEnv(aws.SecretAccessKey)
		aws.DefaultZone = os.ExpandEnv(aws.DefaultZone)
		aws.DefaultImage = os.ExpandEnv(aws.DefaultImage)
		aws.DefaultUsername = os.ExpandEnv(aws.DefaultUsername)
	}
	if c.Provisioner.DigitalOcean != nil {
		do := c.Provisioner.DigitalOcean
		do.Token = os.ExpandEnv(do.Token)
		do.DefaultRegion = os.ExpandEnv(do.DefaultRegion)
		do.DefaultImage = os.ExpandEnv(do.DefaultImage)
		do.DefaultUsername = os.ExpandEnv(do.DefaultUsername)
	}

	// Setup commands
	for i, cmd := range c.Workers.SetupCommands {
		c.Workers.SetupCommands[i] = os.ExpandEnv(cmd)
	}
}

// validate performs configuration validation
func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}

	if len(c.Etcd.Endpoints) == 0 {
		return fmt.Errorf("etcd.endpoints cannot be empty")
	}

	if err := c.Provisioner.Validate(); err != nil {
		return err
	}

	if c.Workers.MaxWorkers <= 0 {
		return fmt.Errorf("workers.max_workers must be positive")
	}

	return nil
}
