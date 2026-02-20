//go:build darwin

package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPaths returns macOS-conventional paths using ~/Library/.
func DefaultPaths() (*Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	supportDir := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy")
	return &Paths{
		SupportDir: supportDir,
		SocketPath: filepath.Join(supportDir, "paw-proxy.sock"),
		CAPath:     filepath.Join(supportDir, "ca.crt"),
		LogPath:    filepath.Join(homeDir, "Library", "Logs", "paw-proxy.log"),
	}, nil
}
