//go:build linux

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPaths_LinuxDefaults(t *testing.T) {
	// Clear XDG vars to test defaults
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}

	wantSupportDir := filepath.Join(homeDir, ".local", "share", "paw-proxy")
	if p.SupportDir != wantSupportDir {
		t.Errorf("SupportDir = %q, want %q", p.SupportDir, wantSupportDir)
	}

	if p.SocketPath != filepath.Join(wantSupportDir, "paw-proxy.sock") {
		t.Errorf("SocketPath = %q, want suffix paw-proxy.sock", p.SocketPath)
	}

	if p.CAPath != filepath.Join(wantSupportDir, "ca.crt") {
		t.Errorf("CAPath = %q, want suffix ca.crt", p.CAPath)
	}

	wantLogPath := filepath.Join(homeDir, ".local", "state", "paw-proxy", "paw-proxy.log")
	if p.LogPath != wantLogPath {
		t.Errorf("LogPath = %q, want %q", p.LogPath, wantLogPath)
	}
}

func TestDefaultPaths_LinuxCustomXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	t.Setenv("XDG_STATE_HOME", "")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	wantSupportDir := filepath.Join("/custom/data", "paw-proxy")
	if p.SupportDir != wantSupportDir {
		t.Errorf("SupportDir = %q, want %q", p.SupportDir, wantSupportDir)
	}

	if p.SocketPath != filepath.Join(wantSupportDir, "paw-proxy.sock") {
		t.Errorf("SocketPath = %q, want %q", p.SocketPath, filepath.Join(wantSupportDir, "paw-proxy.sock"))
	}

	if p.CAPath != filepath.Join(wantSupportDir, "ca.crt") {
		t.Errorf("CAPath = %q, want %q", p.CAPath, filepath.Join(wantSupportDir, "ca.crt"))
	}
}

func TestDefaultPaths_LinuxCustomXDGStateHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	wantLogPath := filepath.Join("/custom/state", "paw-proxy", "paw-proxy.log")
	if p.LogPath != wantLogPath {
		t.Errorf("LogPath = %q, want %q", p.LogPath, wantLogPath)
	}
}

func TestDefaultPaths_LinuxBothXDGCustom(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/opt/data")
	t.Setenv("XDG_STATE_HOME", "/opt/state")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	if p.SupportDir != filepath.Join("/opt/data", "paw-proxy") {
		t.Errorf("SupportDir = %q, want /opt/data/paw-proxy", p.SupportDir)
	}
	if p.LogPath != filepath.Join("/opt/state", "paw-proxy", "paw-proxy.log") {
		t.Errorf("LogPath = %q, want /opt/state/paw-proxy/paw-proxy.log", p.LogPath)
	}
}

func TestDefaultPaths_LinuxPathsAreAbsolute(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	for name, path := range map[string]string{
		"SupportDir": p.SupportDir,
		"SocketPath": p.SocketPath,
		"CAPath":     p.CAPath,
		"LogPath":    p.LogPath,
	} {
		if !filepath.IsAbs(path) {
			t.Errorf("%s = %q is not absolute", name, path)
		}
	}
}

func TestDefaultPaths_LinuxSocketAndCAInsideSupportDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	if !strings.HasPrefix(p.SocketPath, p.SupportDir) {
		t.Errorf("SocketPath %q not inside SupportDir %q", p.SocketPath, p.SupportDir)
	}

	if !strings.HasPrefix(p.CAPath, p.SupportDir) {
		t.Errorf("CAPath %q not inside SupportDir %q", p.CAPath, p.SupportDir)
	}
}

func TestDefaultPaths_LinuxLogNotInsideSupportDir(t *testing.T) {
	// On Linux, log goes under XDG_STATE_HOME while data goes under
	// XDG_DATA_HOME — they should be in different directory trees.
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	if strings.HasPrefix(p.LogPath, p.SupportDir) {
		t.Errorf("LogPath %q should NOT be inside SupportDir %q (XDG separates data and state)", p.LogPath, p.SupportDir)
	}
}
