package targets

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"reconswarm/internal/logging"

	"go.uber.org/zap"
)

// CrtshResult represents a single certificate entry from crt.sh
type CrtshResult struct {
	ID             int    `json:"id"`
	IssuerCAID     int    `json:"issuer_ca_id"`
	IssuerName     string `json:"issuer_name"`
	CommonName     string `json:"common_name"`
	NameValue      string `json:"name_value"`
	EntryTimestamp string `json:"entry_timestamp"`
	NotBefore      string `json:"not_before"`
	NotAfter       string `json:"not_after"`
	SerialNumber   string `json:"serial_number"`
}

// CrtshClient defines the interface for crt.sh operations
type CrtshClient interface {
	Dump(domain string) ([]string, error)
}

// DefaultCrtshClient is the default implementation that makes real HTTP requests
type DefaultCrtshClient struct{}

// MockCrtshClient is a mock implementation for testing
type MockCrtshClient struct {
	Results map[string][]string
}

// NewMockCrtshClient creates a new mock crt.sh client
func NewMockCrtshClient() *MockCrtshClient {
	return &MockCrtshClient{
		Results: make(map[string][]string),
	}
}

// SetMockResults sets mock results for a domain
func (m *MockCrtshClient) SetMockResults(domain string, subdomains []string) {
	m.Results[domain] = subdomains
}

// Dump implements the CrtshClient interface for mock
func (m *MockCrtshClient) Dump(domain string) ([]string, error) {
	if results, exists := m.Results[domain]; exists {
		return results, nil
	}
	return []string{}, nil
}

// Dump implements the CrtshClient interface for default client
func (d *DefaultCrtshClient) Dump(domain string) ([]string, error) {
	return CrtshDump(domain)
}

// Global client instance
var crtshClient CrtshClient = &DefaultCrtshClient{}

// SetCrtshClient sets the global crt.sh client (useful for testing)
func SetCrtshClient(client CrtshClient) {
	crtshClient = client
}

// GetCrtshClient returns the current crt.sh client
func GetCrtshClient() CrtshClient {
	return crtshClient
}

// CrtshDump fetches subdomains from crt.sh for a given domain and filters them by DNS resolution
func CrtshDump(domain string) ([]string, error) {
	logging.Logger().Info("Starting crt.sh subdomain enumeration",
		zap.String("domain", domain))

	// Construct the crt.sh API URL
	url := fmt.Sprintf("https://crt.sh/?q=%s&output=json", domain)
	logging.Logger().Debug("Constructed crt.sh URL", zap.String("url", url))

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create HTTP request with User-Agent header
	logging.Logger().Debug("Making HTTP request to crt.sh")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logging.Logger().Error("Failed to create HTTP request", zap.Error(err))
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add User-Agent header
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		logging.Logger().Error("Failed to fetch data from crt.sh", zap.Error(err))
		return nil, fmt.Errorf("failed to fetch data from crt.sh: %w", err)
	}
	defer resp.Body.Close()

	logging.Logger().Debug("Received response from crt.sh",
		zap.Int("status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		logging.Logger().Error("crt.sh returned non-OK status",
			zap.Int("status_code", resp.StatusCode))
		return nil, fmt.Errorf("crt.sh returned status code: %d", resp.StatusCode)
	}

	// Parse JSON response
	logging.Logger().Debug("Parsing JSON response from crt.sh")
	var results []CrtshResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		logging.Logger().Error("Failed to parse JSON response", zap.Error(err))
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	logging.Logger().Debug("Parsed crt.sh response",
		zap.Int("certificates_found", len(results)))

	// Extract unique subdomains
	subdomains := make(map[string]bool)
	for _, result := range results {
		// Parse name_value field which can contain multiple domains separated by newlines
		names := strings.Split(result.NameValue, "\n")
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" && strings.Contains(name, domain) {
				// Remove wildcard prefix if present
				name = strings.TrimPrefix(name, "*.")
				// Only include subdomains (not the exact domain)
				if name != domain && strings.HasSuffix(name, "."+domain) {
					subdomains[name] = true
				}
			}
		}
	}

	// Convert map keys to slice
	var uniqueSubdomains []string
	for subdomain := range subdomains {
		uniqueSubdomains = append(uniqueSubdomains, subdomain)
	}

	logging.Logger().Debug("Starting DNS resolution filtering",
		zap.Int("unique_subdomains", len(uniqueSubdomains)))

	// Filter subdomains by DNS resolution
	var resolvedSubdomains []string
	for _, subdomain := range uniqueSubdomains {
		if isResolvable(subdomain) {
			resolvedSubdomains = append(resolvedSubdomains, subdomain)
			logging.Logger().Debug("Subdomain resolved",
				zap.String("subdomain", subdomain))
		} else {
			logging.Logger().Debug("Subdomain did not resolve",
				zap.String("subdomain", subdomain))
		}
	}

	logging.Logger().Info("Subdomain enumeration completed",
		zap.String("domain", domain),
		zap.Int("total_found", len(uniqueSubdomains)),
		zap.Int("resolved", len(resolvedSubdomains)))

	return resolvedSubdomains, nil
}

// isResolvable checks if a domain name can be resolved to an IP address
func isResolvable(domain string) bool {
	// Try to resolve the domain
	ips, err := net.LookupIP(domain)
	if err != nil {
		return false
	}

	// Check if we got at least one valid IP
	return len(ips) > 0
}
