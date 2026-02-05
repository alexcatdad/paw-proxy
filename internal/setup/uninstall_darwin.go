//go:build darwin

// internal/setup/uninstall_darwin.go
package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Uninstall(supportDir, tld string, fromBrew bool) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "dev.paw-proxy.plist")
	resolverPath := filepath.Join("/etc/resolver", tld)

	var errs []string

	fmt.Println("paw-proxy uninstall")
	fmt.Println("===================")

	// 1. Stop and remove LaunchAgent
	fmt.Printf("\n[1/3] Removing daemon...\n")
	if err := exec.Command("launchctl", "unload", plistPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not unload LaunchAgent: %v\n", err)
		// Not fatal — agent may not be loaded
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("removing LaunchAgent plist: %v", err))
		fmt.Fprintf(os.Stderr, "  warning: could not remove plist: %v\n", err)
	} else {
		fmt.Printf("  LaunchAgent removed\n")
	}

	// 2. Remove resolver
	fmt.Printf("\n[2/3] Removing DNS resolver...\n")
	if err := os.Remove(resolverPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("removing resolver file: %v", err))
		fmt.Fprintf(os.Stderr, "  warning: could not remove /etc/resolver/%s: %v\n", tld, err)
	} else {
		fmt.Printf("  /etc/resolver/%s removed\n", tld)
	}

	// 3. Remove CA (prompt unless --brew)
	removeCA := fromBrew
	if !fromBrew {
		fmt.Printf("\n[3/3] Remove CA certificate from keychain? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		removeCA = strings.ToLower(strings.TrimSpace(answer)) == "y"
	}

	if removeCA {
		// SECURITY: Use explicit exec.Command args instead of sh -c to prevent shell injection
		keychainPath := filepath.Join(homeDir, "Library", "Keychains", "login.keychain-db")
		out, err := exec.Command("security", "find-certificate", "-a", "-c", "paw-proxy CA", "-Z", keychainPath).Output()
		if err != nil {
			// No certs found is not an error — may have been removed already
			fmt.Println("  No CA certificates found in keychain")
		} else {
			// Parse SHA-1 hashes from output
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "SHA-1 hash:") {
					sha := strings.TrimSpace(strings.TrimPrefix(line, "SHA-1 hash:"))
					if sha != "" {
						if err := exec.Command("security", "delete-certificate", "-Z", sha, keychainPath).Run(); err != nil {
							errs = append(errs, fmt.Sprintf("removing cert %s: %v", sha[:8], err))
							fmt.Fprintf(os.Stderr, "  warning: could not remove certificate %s: %v\n", sha[:8], err)
						} else {
							fmt.Printf("  Removed certificate %s\n", sha[:8])
						}
					}
				}
			}
		}

		// Remove support directory
		if err := os.RemoveAll(supportDir); err != nil {
			errs = append(errs, fmt.Sprintf("removing support directory: %v", err))
			fmt.Fprintf(os.Stderr, "  warning: could not remove support directory: %v\n", err)
		} else {
			fmt.Printf("  Support directory removed\n")
		}
	} else {
		fmt.Printf("  CA kept in keychain\n")
	}

	fmt.Println("\n===================")

	if len(errs) > 0 {
		fmt.Println("Uninstall completed with errors.")
		return fmt.Errorf("uninstall completed with errors: %s", strings.Join(errs, "; "))
	}

	fmt.Println("Uninstall complete!")
	return nil
}
