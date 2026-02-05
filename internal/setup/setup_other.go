//go:build !darwin

package setup

import "fmt"

type Config struct {
	SupportDir string
	BinaryPath string
	DNSPort    int
	TLD        string
}

func Run(config *Config) error {
	return fmt.Errorf("paw-proxy only supports macOS")
}

func Uninstall(supportDir, tld string, fromBrew bool) error {
	return fmt.Errorf("paw-proxy only supports macOS")
}
