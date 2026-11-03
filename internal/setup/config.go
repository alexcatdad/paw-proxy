package setup

// Config holds platform-independent configuration for paw-proxy setup.
type Config struct {
	SupportDir string
	BinaryPath string
	DNSPort    int
	TLD        string

	// LegacyLaunchAgent forces the CLI-written ~/Library/LaunchAgents
	// agent (dev.paw-proxy) instead of registration through Paw Proxy.app.
	// macOS only; kept for development and the transition period.
	LegacyLaunchAgent bool
}
