package setup

// Config holds platform-independent configuration for paw-proxy setup.
type Config struct {
	SupportDir string
	BinaryPath string
	DNSPort    int
	TLD        string
}
