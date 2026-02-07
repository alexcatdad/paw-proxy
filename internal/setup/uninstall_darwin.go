//go:build darwin

// internal/setup/uninstall_darwin.go
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "dev.paw-proxy.plist")
	resolverPath := filepath.Join("/etc/resolver", tld)

	var errs []error

	fmt.Println("paw-proxy uninstall")
	fmt.Println("===================")

	// 1. Stop and remove LaunchAgent (bootout releases socket reservations, unlike unload)
	fmt.Printf("\n[1/3] Removing daemon...\n")
	if err := launchctlBootout(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not bootout LaunchAgent: %v\n", err)
		// Not fatal — agent may not be loaded
	}
	if err := os.Remove(plistPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  LaunchAgent not found (already removed)\n")
		} else {
			errs = append(errs, fmt.Errorf("removing LaunchAgent plist: %w", err))
			fmt.Fprintf(os.Stderr, "  warning: could not remove plist: %v\n", err)
		}
	} else {
		fmt.Printf("  LaunchAgent removed\n")
	}

	// 2. Remove resolver
	fmt.Printf("\n[2/3] Removing DNS resolver...\n")
	if err := os.Remove(resolverPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  /etc/resolver/%s not found (already removed)\n", tld)
		} else {
			errs = append(errs, fmt.Errorf("removing resolver file: %w", err))
			fmt.Fprintf(os.Stderr, "  warning: could not remove /etc/resolver/%s: %v\n", tld, err)
		}
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
		out, err := exec.Command("security", "find-certificate", "-a", "-c", "paw-proxy CA", "-Z", keychainPath).CombinedOutput()
		if err != nil {
			// SECURITY: Exit code 44 = errSecItemNotFound (macOS OSStatus -25300).
			// Only treat this specific code as "not found"; other errors (permission
			// denied, keychain locked/corrupted) are real failures.
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
				fmt.Println("  No CA certificates found in keychain")
			} else {
				errs = append(errs, fmt.Errorf("finding CA certificates: %w", err))
				fmt.Fprintf(os.Stderr, "  warning: security find-certificate failed: %s\n", strings.TrimSpace(string(out)))
			}
		} else {
			// Parse SHA-1 hashes from output
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "SHA-1 hash:") {
					sha := strings.TrimSpace(strings.TrimPrefix(line, "SHA-1 hash:"))
					if sha != "" {
						short := sha
						if len(short) > 8 {
							short = short[:8]
						}
						if err := exec.Command("security", "delete-certificate", "-Z", sha, keychainPath).Run(); err != nil {
							errs = append(errs, fmt.Errorf("removing cert %s: %w", short, err))
							fmt.Fprintf(os.Stderr, "  warning: could not remove certificate %s: %v\n", short, err)
						} else {
							fmt.Printf("  Removed certificate %s\n", short)
						}
					}
				}
			}
		}

		// Remove support directory
		if err := os.RemoveAll(supportDir); err != nil {
			errs = append(errs, fmt.Errorf("removing support directory: %w", err))
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
		return fmt.Errorf("uninstall completed with errors: %w", errors.Join(errs...))
	}

	fmt.Println("Uninstall complete!")
	return nil
}
