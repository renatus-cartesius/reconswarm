package provisioning

import (
	"fmt"

	"reconswarm/internal/config"
)

// NewProvisioner creates a provisioner based on config type (factory pattern).
// This implements the discriminated union dispatch.
func NewProvisioner(cfg config.ProvisionerConfig) (Provisioner, error) {
	switch cfg.Type {
	case config.ProviderYandexCloud:
		if cfg.YandexCloud == nil {
			return nil, fmt.Errorf("yandex_cloud config is nil")
		}
		return NewYcProvisioner(cfg.YandexCloud.IAMToken, cfg.YandexCloud.FolderID)

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

