package targets

import (
	"testing"
)

func TestMockCrtshClient(t *testing.T) {
	// Create mock client
	mockClient := NewMockCrtshClient()

	// Set up mock results
	testDomain := "test.com"
	expectedSubdomains := []string{"www.test.com", "api.test.com", "admin.test.com"}
	mockClient.SetMockResults(testDomain, expectedSubdomains)

	// Test the mock client
	results, err := mockClient.Dump(testDomain)
	if err != nil {
		t.Fatalf("Mock client Dump failed: %v", err)
	}

	// Verify results
	if len(results) != len(expectedSubdomains) {
		t.Errorf("Expected %d subdomains, got %d", len(expectedSubdomains), len(results))
	}

	for i, expected := range expectedSubdomains {
		if results[i] != expected {
			t.Errorf("Expected subdomain %d to be '%s', got '%s'", i, expected, results[i])
		}
	}
}

func TestMockCrtshClient_UnknownDomain(t *testing.T) {
	// Create mock client
	mockClient := NewMockCrtshClient()

	// Test with unknown domain
	unknownDomain := "unknown.com"
	results, err := mockClient.Dump(unknownDomain)
	if err != nil {
		t.Fatalf("Mock client Dump failed: %v", err)
	}

	// Should return empty results for unknown domain
	if len(results) != 0 {
		t.Errorf("Expected empty results for unknown domain, got %d results", len(results))
	}
}

func TestSetCrtshClient(t *testing.T) {
	// Create mock client
	mockClient := NewMockCrtshClient()
	mockClient.SetMockResults("test.com", []string{"www.test.com"})

	// Set the global client
	originalClient := GetCrtshClient()
	SetCrtshClient(mockClient)
	defer SetCrtshClient(originalClient) // Restore original client

	// Test that the global client is now the mock
	currentClient := GetCrtshClient()
	if currentClient != mockClient {
		t.Error("Global client was not set correctly")
	}

	// Test that the mock client works through the global interface
	results, err := GetCrtshClient().Dump("test.com")
	if err != nil {
		t.Fatalf("Global client Dump failed: %v", err)
	}

	if len(results) != 1 || results[0] != "www.test.com" {
		t.Errorf("Expected ['www.test.com'], got %v", results)
	}
}

