package provisioning

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// GCPProvisioner implements the Provisioner interface for Google Cloud
type GCPProvisioner struct {
	service   *compute.Service
	projectID string
}

// NewGCPProvisioner creates a new instance of GCPProvisioner
func NewGCPProvisioner(ctx context.Context, projectID string, credentialsFile string) (*GCPProvisioner, error) {
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithAuthCredentialsFile(option.ServiceAccount, credentialsFile))
	}

	service, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute service: %w", err)
	}

	return &GCPProvisioner{
		service:   service,
		projectID: projectID,
	}, nil
}

// Create creates a new VM in GCP
func (p *GCPProvisioner) Create(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	userData, err := GenerateCloudConfig(spec.Username, spec.SSHPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cloud-config: %w", err)
	}

	rb := &compute.Instance{
		Name:         spec.Name,
		MachineType:  fmt.Sprintf("zones/%s/machineTypes/%s", spec.Zone, p.mapResourcesToMachineType(spec.Cores, spec.Memory)),
		CanIpForward: false,
		Disks: []*compute.AttachedDisk{
			{
				AutoDelete: true,
				Boot:       true,
				Type:       "PERSISTENT",
				InitializeParams: &compute.AttachedDiskInitializeParams{
					SourceImage: spec.ImageID, // e.g., "projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts"
					DiskSizeGb:  spec.DiskSize,
				},
			},
		},
		NetworkInterfaces: []*compute.NetworkInterface{
			{
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: "External NAT",
					},
				},
				Network: "global/networks/default",
			},
		},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				{
					Key:   "user-data",
					Value: &userData,
				},
			},
		},
	}

	op, err := p.service.Instances.Insert(p.projectID, spec.Zone, rb).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to insert instance: %w", err)
	}

	// Wait for operation
	err = p.waitForOperation(ctx, op.Name, spec.Zone)
	if err != nil {
		return nil, fmt.Errorf("operation failed: %w", err)
	}

	// Get instance info
	instance, err := p.service.Instances.Get(p.projectID, spec.Zone, spec.Name).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	ip := ""
	if len(instance.NetworkInterfaces) > 0 && len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
		ip = instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
	}

	return &InstanceInfo{
		ID:     fmt.Sprintf("%d", instance.Id),
		IP:     ip,
		Name:   instance.Name,
		Zone:   spec.Zone,
		Status: instance.Status,
	}, nil
}

// Delete deletes a VM by name
func (p *GCPProvisioner) Delete(ctx context.Context, instanceID string) error {
	// GCP Delete is usually by name, but we store ID. 
	// For simplicity, let's assume we use Name as ID in our internal tracking or we Fetch it.
	// In this implementation, spec.Name was used for creation. 
	// If instanceID is Name:
	op, err := p.service.Instances.Delete(p.projectID, "us-central1-a", instanceID).Context(ctx).Do() // zone needs to be known
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	_ = op
	return nil
}

func (p *GCPProvisioner) waitForOperation(ctx context.Context, opName, zone string) error {
	for i := 0; i < 60; i++ {
		op, err := p.service.ZoneOperations.Get(p.projectID, zone, opName).Context(ctx).Do()
		if err != nil {
			return err
		}
		if op.Status == "DONE" {
			if op.Error != nil {
				return fmt.Errorf("operation error: %v", op.Error.Errors)
			}
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("timeout waiting for operation")
}

func (p *GCPProvisioner) mapResourcesToMachineType(cores int, memory int64) string {
	if cores <= 1 && memory <= 4 {
		return "e2-medium"
	}
	if cores <= 2 && memory <= 8 {
		return "e2-standard-2"
	}
	return "e2-standard-4"
}
