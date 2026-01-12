package provisioning

import (
	"context"
	"fmt"

	"reconswarm/internal/config"
)

// NewProvisioner creates a provisioner based on config type (factory pattern).
// This implements the discriminated union dispatch.
func NewProvisioner(cfg config.ProvisionerConfig) (Provisioner, error) {
	ctx := context.Background()
	switch cfg.Type {
	case config.ProviderYandexCloud:
		if cfg.YandexCloud == nil {
			return nil, fmt.Errorf("yandex_cloud config is nil")
		}
		return NewYcProvisioner(cfg.YandexCloud.IAMToken, cfg.YandexCloud.FolderID)

	case config.ProviderGCP:
		if cfg.GCP == nil {
			return nil, fmt.Errorf("gcp config is nil")
		}
		return NewGCPProvisioner(ctx, cfg.GCP.ProjectID, cfg.GCP.CredentialsPath)

	case config.ProviderAWS:
		if cfg.AWS == nil {
			return nil, fmt.Errorf("aws config is nil")
		}
		return NewAWSProvisioner(ctx, cfg.AWS.Region, cfg.AWS.AccessKeyID, cfg.AWS.SecretAccessKey)

	case config.ProviderDigitalOcean:
		if cfg.DigitalOcean == nil {
			return nil, fmt.Errorf("digitalocean config is nil")
		}
		return NewDOProvisioner(cfg.DigitalOcean.Token)

	default:
		return nil, fmt.Errorf("unsupported provisioner type: %s", cfg.Type)
	}
}

// VMDefaults contains default VM parameters extracted from config
type VMDefaults struct {
	Zone     string
	Image    string
	Username string
	Cores    int
	Memory   int64
	DiskSize int64
}

// GetVMDefaults extracts VM defaults from provisioner config
func GetVMDefaults(cfg config.ProvisionerConfig) VMDefaults {
	switch cfg.Type {
	case config.ProviderYandexCloud:
		if cfg.YandexCloud != nil {
			return VMDefaults{
				Zone:     cfg.YandexCloud.DefaultZone,
				Image:    cfg.YandexCloud.DefaultImage,
				Username: cfg.YandexCloud.DefaultUsername,
				Cores:    cfg.YandexCloud.DefaultCores,
				Memory:   cfg.YandexCloud.DefaultMemory,
				DiskSize: cfg.YandexCloud.DefaultDiskSize,
			}
		}
	case config.ProviderGCP:
		if cfg.GCP != nil {
			return VMDefaults{
				Zone:     cfg.GCP.DefaultZone,
				Image:    cfg.GCP.DefaultImage,
				Username: cfg.GCP.DefaultUsername,
				Cores:    cfg.GCP.DefaultCores,
				Memory:   cfg.GCP.DefaultMemory,
				DiskSize: cfg.GCP.DefaultDiskSize,
			}
		}
	case config.ProviderAWS:
		if cfg.AWS != nil {
			return VMDefaults{
				Zone:     cfg.AWS.DefaultZone,
				Image:    cfg.AWS.DefaultImage,
				Username: cfg.AWS.DefaultUsername,
				Cores:    cfg.AWS.DefaultCores,
				Memory:   cfg.AWS.DefaultMemory,
				DiskSize: cfg.AWS.DefaultDiskSize,
			}
		}
	case config.ProviderDigitalOcean:
		if cfg.DigitalOcean != nil {
			return VMDefaults{
				Zone:     cfg.DigitalOcean.DefaultRegion,
				Image:    cfg.DigitalOcean.DefaultImage,
				Username: cfg.DigitalOcean.DefaultUsername,
				Cores:    cfg.DigitalOcean.DefaultCores,
				Memory:   cfg.DigitalOcean.DefaultMemory,
				DiskSize: cfg.DigitalOcean.DefaultDiskSize,
			}
		}
	}
	// Return sensible defaults
	return VMDefaults{
		Zone:     "ru-central1-b",
		Username: "reconswarm",
		Cores:    2,
		Memory:   2,
		DiskSize: 20,
	}
}


