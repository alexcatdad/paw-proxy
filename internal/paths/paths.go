// Package paths provides platform-aware path resolution for paw-proxy.
// macOS is the supported platform; other platforms get a stub that errors.
package paths

// Paths holds all platform-specific filesystem paths for paw-proxy.
type Paths struct {
	SupportDir string // Data directory (CA certs, socket)
	SocketPath string // Unix domain socket for the control API
	CAPath     string // CA certificate path
	LogPath    string // Daemon log file path
}
