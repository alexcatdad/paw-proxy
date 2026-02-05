// cmd/up/main_test.go
package main

import (
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "converts to lowercase",
			input:    "MyApp",
			expected: "myapp",
		},
		{
			name:     "replaces special characters with dashes",
			input:    "my@app",
			expected: "my-app",
		},
		{
			name:     "trims leading dashes",
			input:    "@scope/pkg",
			expected: "scope-pkg",
		},
		{
			name:     "trims trailing dashes",
			input:    "app@@@",
			expected: "app",
		},
		{
			name:     "handles multiple consecutive special chars",
			input:    "my@@app",
			expected: "my--app",
		},
		{
			name:     "preserves existing dashes",
			input:    "my-app",
			expected: "my-app",
		},
		{
			name:     "handles all special characters",
			input:    "@@@",
			expected: "app",
		},
		{
			name:     "handles scoped package names",
			input:    "@babel/core",
			expected: "babel-core",
		},
		{
			name:     "handles mixed case with special chars",
			input:    "MyApp@2.0",
			expected: "myapp-2-0",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
