//go:build !darwin && !linux

package setup

import "fmt"

func Run(config *Config) error {
	return fmt.Errorf("paw-proxy setup only supports macOS and Linux")
}

func Uninstall(supportDir, tld string, fromBrew bool) error {
	return fmt.Errorf("paw-proxy uninstall only supports macOS and Linux")
}
