package control

import (
	"testing"
)

func TestSSH_GetInstanceName(t *testing.T) {
	// Create SSH instance directly without connection
	ssh := &SSH{
		client:       nil, // We don't need a real connection for this test
		host:         "test-host",
		user:         "test-user",
		instanceName: "test-instance-123",
	}

	// Test GetInstanceName method
	instanceName := ssh.GetInstanceName()
	expected := "test-instance-123"

	if instanceName != expected {
		t.Errorf("Expected instance name '%s', got '%s'", expected, instanceName)
	}
}

func TestSSH_GetInstanceName_Empty(t *testing.T) {
	// Test with empty instance name
	ssh := &SSH{
		client:       nil,
		host:         "test-host",
		user:         "test-user",
		instanceName: "",
	}

	instanceName := ssh.GetInstanceName()
	expected := ""

	if instanceName != expected {
		t.Errorf("Expected empty instance name, got '%s'", instanceName)
	}
}
