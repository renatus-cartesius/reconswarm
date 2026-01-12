package provisioning

import "context"

// InstanceSpec represents the specification for creating a VM
type InstanceSpec struct {
	Name         string
	Cores        int
	Memory       int64 // in GB
	DiskSize     int64 // in GB
	ImageID      string
	Zone         string
	SSHPublicKey string
	Username     string
}

// InstanceInfo contains information about the created VM
type InstanceInfo struct {
	ID     string
	IP     string
	Name   string
	Zone   string
	Status string
}

// Provisioner defines the interface for managing virtual machines
type Provisioner interface {
	Create(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error)
	Delete(ctx context.Context, instanceID string) error
}
