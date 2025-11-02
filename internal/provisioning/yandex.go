package provisioning

import (
	"context"
	"fmt"
	"log"

	"github.com/yandex-cloud/go-genproto/yandex/cloud/compute/v1"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/vpc/v1"
	ycsdk "github.com/yandex-cloud/go-sdk"
)

// YcProvisioner implements the Provisioner interface for Yandex Cloud
type YcProvisioner struct {
	sdk      *ycsdk.SDK
	folderID string
}

// NewYcProvisioner creates a new instance of YcProvisioner
func NewYcProvisioner(iamToken, folderID string) (*YcProvisioner, error) {
	ctx := context.Background()

	// Create SDK client
	sdk, err := ycsdk.Build(ctx, ycsdk.Config{
		Credentials: ycsdk.NewIAMTokenCredentials(iamToken),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SDK: %w", err)
	}

	return &YcProvisioner{
		sdk:      sdk,
		folderID: folderID,
	}, nil
}

// Create creates a new VM in Yandex Cloud
func (p *YcProvisioner) Create(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	// Find subnet if not specified
	subnetID := p.findSubnet(ctx, spec.Zone)
	if subnetID == "" {
		return nil, fmt.Errorf("no subnet found in zone %s", spec.Zone)
	}

	// Get image
	imageID := spec.ImageID
	if imageID == "" {
		imageID = p.getDefaultImage(ctx)
	}

	// Create VM creation request
	request := &compute.CreateInstanceRequest{
		FolderId:   p.folderID,
		Name:       spec.Name,
		ZoneId:     spec.Zone,
		PlatformId: "standard-v1",
		ResourcesSpec: &compute.ResourcesSpec{
			Cores:  int64(spec.Cores),
			Memory: spec.Memory * 1024 * 1024 * 1024,
		},
		BootDiskSpec: &compute.AttachedDiskSpec{
			AutoDelete: true,
			Disk: &compute.AttachedDiskSpec_DiskSpec_{
				DiskSpec: &compute.AttachedDiskSpec_DiskSpec{
					TypeId: "network-hdd",
					Size:   spec.DiskSize * 1024 * 1024 * 1024,
					Source: &compute.AttachedDiskSpec_DiskSpec_ImageId{
						ImageId: imageID,
					},
				},
			},
		},
		NetworkInterfaceSpecs: []*compute.NetworkInterfaceSpec{
			{
				SubnetId: subnetID,
				PrimaryV4AddressSpec: &compute.PrimaryAddressSpec{
					OneToOneNatSpec: &compute.OneToOneNatSpec{
						IpVersion: compute.IpVersion_IPV4,
					},
				},
			},
		},
		Metadata: map[string]string{
			"user-data": p.generateUserData(spec.Username, spec.SSHPublicKey),
		},
	}

	// Create VM
	pop, err := p.sdk.Compute().Instance().Create(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}

	// Wait for operation completion
	op, err := p.sdk.WrapOperation(pop, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap operation: %w", err)
	}

	err = op.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for operation: %w", err)
	}

	// Get result
	resp, err := op.Response()
	if err != nil {
		return nil, fmt.Errorf("failed to get response: %w", err)
	}

	instance := resp.(*compute.Instance)

	// Get IP address
	ip := ""
	if len(instance.NetworkInterfaces) > 0 {
		if nat := instance.NetworkInterfaces[0].PrimaryV4Address.OneToOneNat; nat != nil {
			ip = nat.Address
		}
	}

	return &InstanceInfo{
		ID:     instance.Id,
		IP:     ip,
		Name:   instance.Name,
		Zone:   instance.ZoneId,
		Status: instance.Status.String(),
	}, nil
}

// Delete deletes a VM by ID
func (p *YcProvisioner) Delete(ctx context.Context, instanceID string) error {
	pop, err := p.sdk.Compute().Instance().Delete(ctx, &compute.DeleteInstanceRequest{
		InstanceId: instanceID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	// Wait for operation completion
	op, err := p.sdk.WrapOperation(pop, nil)
	if err != nil {
		return fmt.Errorf("failed to wrap operation: %w", err)
	}

	err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for operation: %w", err)
	}

	return nil
}

// findSubnet finds a subnet in the specified zone
func (p *YcProvisioner) findSubnet(ctx context.Context, zone string) string {
	resp, err := p.sdk.VPC().Subnet().List(ctx, &vpc.ListSubnetsRequest{
		FolderId: p.folderID,
		PageSize: 100,
	})
	if err != nil {
		log.Printf("Error getting subnet list: %v", err)
		return ""
	}

	for _, subnet := range resp.Subnets {
		if subnet.ZoneId == zone {
			return subnet.Id
		}
	}

	return ""
}

// getDefaultImage gets the default image ID
func (p *YcProvisioner) getDefaultImage(ctx context.Context) string {
	image, err := p.sdk.Compute().Image().GetLatestByFamily(ctx, &compute.GetImageLatestByFamilyRequest{
		FolderId: "standard-images",
		Family:   "ubuntu-24-04-lts",
	})
	if err != nil {
		log.Printf("Error getting image: %v", err)
		return "fd82odtq5h79jo7ffss3" // Ubuntu 20.04 as fallback
	}
	return image.Id
}

// generateUserData generates cloud-config user-data for VM creation
func (p *YcProvisioner) generateUserData(username, sshPublicKey string) string {
	userData, err := GenerateCloudConfig(username, sshPublicKey)
	if err != nil {
		log.Printf("Warning: failed to generate user-data: %v", err)
		// Return minimal cloud-config as fallback
		return fmt.Sprintf(`#cloud-config
ssh_pwauth: no
users:
  - name: %s
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh-authorized-keys:
      - "%s"
`, username, sshPublicKey)
	}
	return userData
}
