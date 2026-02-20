//go:build linux

package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPaths returns XDG-compliant paths for Linux.
// Respects XDG_DATA_HOME and XDG_STATE_HOME if set.
func DefaultPaths() (*Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(homeDir, ".local", "state")
	}

	supportDir := filepath.Join(dataHome, "paw-proxy")
	return &Paths{
		SupportDir: supportDir,
		SocketPath: filepath.Join(supportDir, "paw-proxy.sock"),
		CAPath:     filepath.Join(supportDir, "ca.crt"),
		LogPath:    filepath.Join(stateHome, "paw-proxy", "paw-proxy.log"),
	}, nil
}
