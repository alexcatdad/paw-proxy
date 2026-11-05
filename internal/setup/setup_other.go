//go:build !darwin

package setup

import "fmt"

func Run(config *Config) error {
	return fmt.Errorf("paw-proxy setup only supports macOS")
}

func Uninstall(supportDir, tld string, fromBrew bool) error {
	return fmt.Errorf("paw-proxy uninstall only supports macOS")
}
