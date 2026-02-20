//go:build linux

package setup

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Uninstall(supportDir, tld string, fromBrew bool) error {
	homeDir, err := realUserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	unitPath := filepath.Join(homeDir, ".config", "systemd", "user", "paw-proxy.service")
	resolvedConf := filepath.Join("/etc/systemd/resolved.conf.d", "paw-proxy.conf")

	var errs []error

	fmt.Println("paw-proxy uninstall")
	fmt.Println("===================")

	// 1. Stop and remove systemd user service
	fmt.Printf("\n[1/3] Removing daemon...\n")
	if err := systemctlAsUser("disable", "--now", "paw-proxy"); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not disable service: %v\n", err)
		// Not fatal — service may not be loaded
	}
	if err := systemctlAsUser("daemon-reload"); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: daemon-reload failed: %v\n", err)
	}
	if err := os.Remove(unitPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  Service unit not found (already removed)\n")
		} else {
			errs = append(errs, fmt.Errorf("removing unit file: %w", err))
			fmt.Fprintf(os.Stderr, "  warning: could not remove unit file: %v\n", err)
		}
	} else {
		fmt.Printf("  Systemd service removed\n")
	}

	// 2. Remove DNS resolver config
	fmt.Printf("\n[2/3] Removing DNS resolver...\n")
	if err := os.Remove(resolvedConf); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  Resolved config not found (already removed)\n")
		} else {
			errs = append(errs, fmt.Errorf("removing resolved config: %w", err))
			fmt.Fprintf(os.Stderr, "  warning: could not remove %s: %v\n", resolvedConf, err)
		}
	} else {
		fmt.Printf("  systemd-resolved config removed\n")
	}
	// Restart resolved to pick up the change
	if err := exec.Command("systemctl", "restart", "systemd-resolved").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not restart systemd-resolved: %v\n", err)
	}

	// 3. Remove CA and support directory
	removeCA := fromBrew
	if !fromBrew {
		fmt.Printf("\n[3/3] Remove CA certificate from system trust? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		removeCA = strings.ToLower(strings.TrimSpace(answer)) == "y"
	}

	if removeCA {
		// Remove from Debian/Ubuntu trust store
		debianPath := "/usr/local/share/ca-certificates/paw-proxy-ca.crt"
		if err := os.Remove(debianPath); err == nil {
			fmt.Printf("  Removed CA from %s\n", debianPath)
			if cmd := exec.Command("update-ca-certificates"); cmd.Run() != nil {
				fmt.Fprintf(os.Stderr, "  warning: update-ca-certificates failed\n")
			}
		} else if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("removing %s: %w", debianPath, err))
		}

		// Remove from Fedora/RHEL/Arch trust store
		fedoraPath := "/etc/pki/ca-trust/source/anchors/paw-proxy-ca.crt"
		if err := os.Remove(fedoraPath); err == nil {
			fmt.Printf("  Removed CA from %s\n", fedoraPath)
			if cmd := exec.Command("update-ca-trust"); cmd.Run() != nil {
				fmt.Fprintf(os.Stderr, "  warning: update-ca-trust failed\n")
			}
		} else if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("removing %s: %w", fedoraPath, err))
		}

		// Remove support directory
		if err := os.RemoveAll(supportDir); err != nil {
			errs = append(errs, fmt.Errorf("removing support directory: %w", err))
			fmt.Fprintf(os.Stderr, "  warning: could not remove support directory: %v\n", err)
		} else {
			fmt.Printf("  Support directory removed\n")
		}
	} else {
		fmt.Printf("  CA kept in system trust store\n")
	}

	fmt.Println("\n===================")

	if len(errs) > 0 {
		fmt.Println("Uninstall completed with errors.")
		return fmt.Errorf("uninstall completed with errors: %w", errors.Join(errs...))
	}

	fmt.Println("Uninstall complete!")
	return nil
}
