//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/alexcatdad/paw-proxy/internal/appbundle"
)

func restartService() error {
	uid := os.Getuid()
	args, err := restartArgs(uid, func(target string) bool {
		return exec.Command("launchctl", "print", target).Run() == nil
	})
	if err != nil {
		return err
	}
	if out, err := exec.Command("launchctl", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl %v: %v: %s", args, err, out)
	}
	return nil
}

// restartArgs picks the loaded agent, preferring the app-owned one over the
// legacy CLI-written agent, and returns the launchctl kickstart arguments.
func restartArgs(uid int, loaded func(target string) bool) ([]string, error) {
	for _, label := range []string{appbundle.DaemonLabel, appbundle.LegacyLabel} {
		target := fmt.Sprintf("gui/%d/%s", uid, label)
		if loaded(target) {
			return []string{"kickstart", "-k", target}, nil
		}
	}
	return nil, errNoService
}
