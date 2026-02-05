// cmd/up/main_test.go
package main

import (
	"testing"
)

func TestExtractConflictDir(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "conflict error with directory",
			err:      &conflictError{dir: "/path/to/existing/project"},
			expected: "/path/to/existing/project",
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "other error type",
			err:      &testError{msg: "some other error"},
			expected: "",
		},
		{
			name:     "conflict error with empty directory",
			err:      &conflictError{dir: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractConflictDir(tt.err)
			if result != tt.expected {
				t.Errorf("extractConflictDir() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConflictError(t *testing.T) {
	err := &conflictError{dir: "/Users/test/myproject"}
	expected := "conflict: route already registered from /Users/test/myproject"

	if err.Error() != expected {
		t.Errorf("conflictError.Error() = %q, want %q", err.Error(), expected)
	}
}

// testError is a mock error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

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
