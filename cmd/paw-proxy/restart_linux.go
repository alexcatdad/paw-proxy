//go:build linux

package main

import (
	"fmt"
	"os/exec"
)

func restartService() error {
	if exec.Command("systemctl", "--user", "cat", "paw-proxy.service").Run() != nil {
		return errNoService
	}
	if out, err := exec.Command("systemctl", "--user", "restart", "paw-proxy.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user restart: %v: %s", err, out)
	}
	return nil
}
