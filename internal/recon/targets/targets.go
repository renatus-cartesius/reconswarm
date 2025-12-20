package targets

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"

	"reconswarm/internal/logging"

	"go.uber.org/zap"
)

// FromCrtsh fetches subdomains from crt.sh for the given domain.
// Returns the list of discovered subdomains.
func FromCrtsh(domain string) ([]string, error) {
	return GetCrtshClient().Dump(domain)
}

// FromList parses a list of targets from a generic value (typically []any from YAML).
// Returns extracted string targets.
func FromList(value any) []string {
	list, ok := value.([]any)
	if !ok {
		logging.Logger().Error("invalid type for list targets", zap.Any("value", value))
		return nil
	}

	var targets []string
	for _, v := range list {
		if s, ok := v.(string); ok {
			targets = append(targets, s)
		} else {
			logging.Logger().Error("non-string entry in target list", zap.Any("entry", v))
		}
	}
	return targets
}

// FromExternalList downloads a list from HTTP(S) URL and returns targets line by line.
// Uses streaming to handle large lists efficiently without loading everything into memory.
func FromExternalList(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch external list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var targets []string
	scanner := bufio.NewScanner(resp.Body)

	// Increase buffer size for potentially long lines
	const maxLineSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			targets = append(targets, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading external list: %w", err)
	}

	return targets, nil
}
