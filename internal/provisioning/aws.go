package provisioning

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// AWSProvisioner implements the Provisioner interface for AWS
type AWSProvisioner struct {
	client *ec2.Client
}

// NewAWSProvisioner creates a new instance of AWSProvisioner
func NewAWSProvisioner(ctx context.Context, region, accessKey, secretKey string) (*AWSProvisioner, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := ec2.NewFromConfig(cfg)
	return &AWSProvisioner{
		client: client,
	}, nil
}

// Create creates a new EC2 instance
func (p *AWSProvisioner) Create(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	userData, err := GenerateCloudConfig(spec.Username, spec.SSHPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cloud-config: %w", err)
	}
	encodedUserData := base64.StdEncoding.EncodeToString([]byte(userData))

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(spec.ImageID),
		InstanceType: p.mapResourcesToInstanceType(spec.Cores, spec.Memory),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		UserData:     aws.String(encodedUserData),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String(spec.Name)},
				},
			},
		},
	}

	output, err := p.client.RunInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to run instance: %w", err)
	}

	instanceID := output.Instances[0].InstanceId

	// Wait for instance to be running
	for i := 0; i < 60; i++ {
		desc, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{*instanceID},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe instance: %w", err)
		}

		inst := desc.Reservations[0].Instances[0]
		if inst.State.Name == types.InstanceStateNameRunning {
			return &InstanceInfo{
				ID:     *inst.InstanceId,
				IP:     aws.ToString(inst.PublicIpAddress),
				Name:   spec.Name,
				Zone:   aws.ToString(inst.Placement.AvailabilityZone),
				Status: string(inst.State.Name),
			}, nil
		}
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("timed out waiting for instance to be running")
}

// Delete deletes an EC2 instance
func (p *AWSProvisioner) Delete(ctx context.Context, instanceID string) error {
	_, err := p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("failed to terminate instance: %w", err)
	}
	return nil
}

func (p *AWSProvisioner) mapResourcesToInstanceType(cores int, memory int64) types.InstanceType {
	if cores <= 1 && memory <= 2 {
		return types.InstanceTypeT3Micro
	}
	if cores <= 2 && memory <= 4 {
		return types.InstanceTypeT3Small
	}
	return types.InstanceTypeT3Medium
}
