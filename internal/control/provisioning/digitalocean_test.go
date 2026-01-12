package provisioning

import "testing"

func TestDOProvisioner_mapResourcesToSize(t *testing.T) {
	p := &DOProvisioner{}
	tests := []struct {
		cores  int
		memory int64
		want   string
	}{
		{1, 2, "s-1vcpu-2gb"},
		{2, 4, "s-2vcpu-4gb"},
		{4, 8, "s-4vcpu-8gb"},
		{1, 2, "s-1vcpu-2gb"},
	}
	for _, tt := range tests {
		if got := p.mapResourcesToSize(tt.cores, tt.memory); got != tt.want {
			t.Errorf("mapResourcesToSize(%v, %v) = %v, want %v", tt.cores, tt.memory, got, tt.want)
		}
	}
}
