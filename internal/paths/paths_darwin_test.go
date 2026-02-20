//go:build darwin

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPaths_Darwin(t *testing.T) {
	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}

	wantSupportDir := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy")
	if p.SupportDir != wantSupportDir {
		t.Errorf("SupportDir = %q, want %q", p.SupportDir, wantSupportDir)
	}

	if p.SocketPath != filepath.Join(wantSupportDir, "paw-proxy.sock") {
		t.Errorf("SocketPath = %q, want suffix paw-proxy.sock", p.SocketPath)
	}

	if p.CAPath != filepath.Join(wantSupportDir, "ca.crt") {
		t.Errorf("CAPath = %q, want suffix ca.crt", p.CAPath)
	}

	wantLogPath := filepath.Join(homeDir, "Library", "Logs", "paw-proxy.log")
	if p.LogPath != wantLogPath {
		t.Errorf("LogPath = %q, want %q", p.LogPath, wantLogPath)
	}
}

func TestDefaultPaths_DarwinPathsAreAbsolute(t *testing.T) {
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

func TestDefaultPaths_DarwinSocketInsideSupportDir(t *testing.T) {
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
