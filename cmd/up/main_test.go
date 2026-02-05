package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MyApp", "myapp"},
		{"@scope/pkg", "scope-pkg"},
		{"---", "app"},
		{"", "app"},
		{"UPPER", "upper"},
		{"my-app", "my-app"},
		{"my_app", "my-app"},
		{"Hello World", "hello-world"},
		{"123", "123"},
		{"a", "a"},
		{"My.App.Name", "my-app-name"},
		{"--leading-trailing--", "leading-trailing"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractConflictDir(t *testing.T) {
	t.Run("conflict error returns dir", func(t *testing.T) {
		err := &conflictError{dir: "/home/user/project"}
		got := extractConflictDir(err)
		if got != "/home/user/project" {
			t.Errorf("extractConflictDir() = %q, want %q", got, "/home/user/project")
		}
	})

	t.Run("wrapped conflict error returns dir", func(t *testing.T) {
		err := fmt.Errorf("registration failed: %w", &conflictError{dir: "/tmp/app"})
		got := extractConflictDir(err)
		if got != "/tmp/app" {
			t.Errorf("extractConflictDir() = %q, want %q", got, "/tmp/app")
		}
	})

	t.Run("non-conflict error returns empty", func(t *testing.T) {
		err := errors.New("some other error")
		got := extractConflictDir(err)
		if got != "" {
			t.Errorf("extractConflictDir() = %q, want %q", got, "")
		}
	})

	t.Run("nil error returns empty", func(t *testing.T) {
		got := extractConflictDir(nil)
		if got != "" {
			t.Errorf("extractConflictDir(nil) = %q, want %q", got, "")
		}
	})
}
