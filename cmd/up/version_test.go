package main

import "testing"

func TestVersionString(t *testing.T) {
	oldVersion, oldBuild := version, build
	defer func() { version, build = oldVersion, oldBuild }()

	version, build = "2026.11.1", ""
	if got := versionString("up"); got != "up version 2026.11.1" {
		t.Errorf("without build: %q", got)
	}
	version, build = "2026.11.1", "7"
	if got := versionString("up"); got != "up version 2026.11.1 (build 7)" {
		t.Errorf("with build: %q", got)
	}
}
