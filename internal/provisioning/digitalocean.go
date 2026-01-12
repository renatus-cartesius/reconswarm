package provisioning

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/godo"
)

// DOProvisioner implements the Provisioner interface for DigitalOcean
type DOProvisioner struct {
	client *godo.Client
}

// NewDOProvisioner creates a new instance of DOProvisioner
func NewDOProvisioner(token string) (*DOProvisioner, error) {
	client := godo.NewFromToken(token)
	return &DOProvisioner{
		client: client,
	}, nil
}

// Create creates a new droplet in DigitalOcean
func (p *DOProvisioner) Create(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	userData, err := GenerateCloudConfig(spec.Username, spec.SSHPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cloud-config: %w", err)
	}

	createRequest := &godo.DropletCreateRequest{
		Name:   spec.Name,
		Region: spec.Zone,
		Size:   p.mapResourcesToSize(spec.Cores, spec.Memory),
		Image: godo.DropletCreateImage{
			Slug: spec.ImageID,
		},
		UserData: userData,
	}

	droplet, _, err := p.client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create droplet: %w", err)
	}

	// Wait for droplet to be active
	for i := 0; i < 60; i++ {
		d, _, err := p.client.Droplets.Get(ctx, droplet.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get droplet: %w", err)
		}

		if d.Status == "active" {
			ip, _ := d.PublicIPv4()
			return &InstanceInfo{
				ID:     fmt.Sprintf("%d", d.ID),
				IP:     ip,
				Name:   d.Name,
				Zone:   d.Region.Slug,
				Status: d.Status,
			}, nil
		}

		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("timed out waiting for droplet to be active")
}

// Delete deletes a droplet by ID
func (p *DOProvisioner) Delete(ctx context.Context, instanceID string) error {
	id := 0
	_, err := fmt.Sscanf(instanceID, "%d", &id)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %w", err)
	}

	_, err = p.client.Droplets.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete droplet: %w", err)
	}

	return nil
}

func (p *DOProvisioner) mapResourcesToSize(cores int, memory int64) string {
	// Simple mapping for demonstration. In real world, this should be more robust.
	if cores <= 1 && memory <= 2 {
		return "s-1vcpu-2gb"
	}
	if cores <= 2 && memory <= 4 {
		return "s-2vcpu-4gb"
	}
	return "s-4vcpu-8gb"
}
