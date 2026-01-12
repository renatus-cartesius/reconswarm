package provisioning

import (
	"testing"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestAWSProvisioner_mapResourcesToInstanceType(t *testing.T) {
	p := &AWSProvisioner{}
	tests := []struct {
		cores  int
		memory int64
		want   types.InstanceType
	}{
		{1, 2, types.InstanceTypeT3Micro},
		{2, 4, types.InstanceTypeT3Small},
		{4, 8, types.InstanceTypeT3Medium},
	}
	for _, tt := range tests {
		if got := p.mapResourcesToInstanceType(tt.cores, tt.memory); got != tt.want {
			t.Errorf("mapResourcesToInstanceType(%v, %v) = %v, want %v", tt.cores, tt.memory, got, tt.want)
		}
	}
}
