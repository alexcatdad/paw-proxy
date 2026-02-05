// cmd/up/main_test.go
package main

import (
	"regexp"
	"testing"
)

// routeNamePattern matches the server-side validation in internal/api/server.go:21
var routeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

func TestSanitizeName_Lowercases(t *testing.T) {
	result := sanitizeName("MyApp")
	expected := "myapp"
	if result != expected {
		t.Errorf("sanitizeName(%q) = %q; want %q", "MyApp", result, expected)
	}
}

func TestSanitizeName_RemovesLeadingDashes(t *testing.T) {
	result := sanitizeName("@scope/pkg")
	if result[0] == '-' {
		t.Errorf("sanitizeName(%q) = %q; starts with dash", "@scope/pkg", result)
	}
	if !routeNamePattern.MatchString(result) {
		t.Errorf("sanitizeName(%q) = %q; does not match routeNamePattern", "@scope/pkg", result)
	}
}

func TestSanitizeName_RemovesTrailingDashes(t *testing.T) {
	result := sanitizeName("app-")
	if len(result) > 0 && result[len(result)-1] == '-' {
		t.Errorf("sanitizeName(%q) = %q; ends with dash", "app-", result)
	}
}

func TestSanitizeName_AllSpecialChars_UsesFallback(t *testing.T) {
	result := sanitizeName("@@@")
	if result == "" || result == "---" || !routeNamePattern.MatchString(result) {
		t.Errorf("sanitizeName(%q) = %q; want non-empty valid name", "@@@", result)
	}
}

func TestSanitizeName_ValidatesAgainstServerPattern(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"MyApp"},
		{"@scope/package"},
		{"my-app"},
		{"app_name"},
		{"123start"},
		{"---"},
		{"@@@"},
		{"UPPERCASE"},
		{"-leading"},
		{"trailing-"},
	}

	for _, tt := range tests {
		result := sanitizeName(tt.input)
		if !routeNamePattern.MatchString(result) {
			t.Errorf("sanitizeName(%q) = %q; does not match routeNamePattern %s", tt.input, result, routeNamePattern)
		}
	}
}
