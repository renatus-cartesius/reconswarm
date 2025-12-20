package logging

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			input:    strings.Repeat("a", MaxLogFieldLength),
			expected: strings.Repeat("a", MaxLogFieldLength),
		},
		{
			name:     "long string truncated",
			input:    strings.Repeat("a", MaxLogFieldLength+100),
			expected: strings.Repeat("a", MaxLogFieldLength) + "...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input)
			if result != tt.expected {
				t.Errorf("Truncate() = %q (len=%d), want %q (len=%d)",
					result, len(result), tt.expected, len(tt.expected))
			}
		})
	}
}

func TestTruncateN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		n        int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			n:        10,
			expected: "hello",
		},
		{
			name:     "long string truncated",
			input:    "hello world",
			n:        5,
			expected: "hello...",
		},
		{
			name:     "exact length unchanged",
			input:    "hello",
			n:        5,
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateN(tt.input, tt.n)
			if result != tt.expected {
				t.Errorf("TruncateN(%q, %d) = %q, want %q",
					tt.input, tt.n, result, tt.expected)
			}
		})
	}
}

func TestTruncateSlice(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		maxItems int
		expected []string
	}{
		{
			name:     "short slice unchanged",
			items:    []string{"a", "b", "c"},
			maxItems: 5,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "exact length unchanged",
			items:    []string{"a", "b", "c"},
			maxItems: 3,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "long slice truncated",
			items:    []string{"a", "b", "c", "d", "e"},
			maxItems: 2,
			expected: []string{"a", "b", "... and 3 more"},
		},
		{
			name:     "empty slice",
			items:    []string{},
			maxItems: 5,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateSlice(tt.items, tt.maxItems)
			if len(result) != len(tt.expected) {
				t.Errorf("TruncateSlice() len = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("TruncateSlice()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-5, "-5"},
		{1000000, "1000000"},
	}

	for _, tt := range tests {
		result := itoa(tt.input)
		if result != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

