package targets

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromCrtsh(t *testing.T) {
	// Set up mock crt.sh client
	mockClient := NewMockCrtshClient()
	mockClient.SetMockResults("example.com", []string{"www.example.com", "api.example.com"})
	SetCrtshClient(mockClient)
	defer SetCrtshClient(&DefaultCrtshClient{}) // Restore default client

	subdomains, err := FromCrtsh("example.com")
	if err != nil {
		t.Fatalf("FromCrtsh failed: %v", err)
	}

	expected := []string{"www.example.com", "api.example.com"}
	if len(subdomains) != len(expected) {
		t.Errorf("Expected %d subdomains, got %d", len(expected), len(subdomains))
	}

	for i, exp := range expected {
		if subdomains[i] != exp {
			t.Errorf("Subdomain %d: expected '%s', got '%s'", i, exp, subdomains[i])
		}
	}
}

func TestFromList(t *testing.T) {
	// Test valid list
	list := []any{"target1.com", "target2.com", "target3.com"}
	result := FromList(list)

	if len(result) != 3 {
		t.Errorf("Expected 3 targets, got %d", len(result))
	}

	for i, expected := range []string{"target1.com", "target2.com", "target3.com"} {
		if result[i] != expected {
			t.Errorf("Target %d: expected '%s', got '%s'", i, expected, result[i])
		}
	}
}

func TestFromList_InvalidType(t *testing.T) {
	// Test with invalid type (not []any)
	result := FromList("not a list")

	if len(result) != 0 {
		t.Errorf("Expected 0 targets for invalid type, got %d", len(result))
	}
}

func TestFromList_MixedTypes(t *testing.T) {
	// Test list with mixed types (some non-strings)
	list := []any{"valid.com", 123, "another.com"}
	result := FromList(list)

	// Should only contain valid strings
	if len(result) != 2 {
		t.Errorf("Expected 2 valid targets, got %d", len(result))
	}
}

func TestFromExternalList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `line1
line2
# comment
line3

line4
`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	targets, err := FromExternalList(server.URL)
	if err != nil {
		t.Fatalf("FromExternalList failed: %v", err)
	}

	expected := []string{"line1", "line2", "line3", "line4"}
	if len(targets) != len(expected) {
		t.Errorf("Expected %d targets, got %d", len(expected), len(targets))
	}

	for i, exp := range expected {
		if targets[i] != exp {
			t.Errorf("Target %d: expected '%s', got '%s'", i, exp, targets[i])
		}
	}
}

func TestFromExternalList_SkipsComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `# header comment
target1.com
# inline comment
target2.com
`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	targets, err := FromExternalList(server.URL)
	if err != nil {
		t.Fatalf("FromExternalList failed: %v", err)
	}

	if len(targets) != 2 {
		t.Errorf("Expected 2 targets (comments skipped), got %d", len(targets))
	}
}

func TestFromExternalList_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FromExternalList(server.URL)
	if err == nil {
		t.Error("Expected error for HTTP 500, got nil")
	}
}

func TestFromExternalList_InvalidURL(t *testing.T) {
	_, err := FromExternalList("http://invalid-url-that-does-not-exist.test")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}
