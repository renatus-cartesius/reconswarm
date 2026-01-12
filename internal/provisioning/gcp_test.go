package provisioning

import "testing"

func TestGCPProvisioner_mapResourcesToMachineType(t *testing.T) {
	p := &GCPProvisioner{}
	tests := []struct {
		cores  int
		memory int64
		want   string
	}{
		{1, 4, "e2-medium"},
		{2, 8, "e2-standard-2"},
		{4, 16, "e2-standard-4"},
	}
	for _, tt := range tests {
		if got := p.mapResourcesToMachineType(tt.cores, tt.memory); got != tt.want {
			t.Errorf("mapResourcesToMachineType(%v, %v) = %v, want %v", tt.cores, tt.memory, got, tt.want)
		}
	}
}
