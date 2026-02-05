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
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "dev.paw-proxy.plist")
	resolverPath := filepath.Join("/etc/resolver", tld)

	fmt.Println("paw-proxy uninstall")
	fmt.Println("===================")

	// 1. Stop and remove LaunchAgent
	fmt.Printf("\n[1/3] Removing daemon...\n")
	exec.Command("launchctl", "unload", plistPath).Run()
	os.Remove(plistPath)
	fmt.Printf("  ✓ LaunchAgent removed\n")

	// 2. Remove resolver
	fmt.Printf("\n[2/3] Removing DNS resolver...\n")
	os.Remove(resolverPath)
	fmt.Printf("  ✓ /etc/resolver/%s removed\n", tld)

	// 3. Remove CA (prompt unless --brew)
	removeCA := fromBrew
	if !fromBrew {
		fmt.Printf("\n[3/3] Remove CA certificate from keychain? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		removeCA = strings.ToLower(strings.TrimSpace(answer)) == "y"
	}

	if removeCA {
		// Remove from keychain
		cmd := exec.Command("sh", "-c", `
			for sha in $(security find-certificate -a -c "paw-proxy CA" -Z | awk '/SHA-1/ {print $3}'); do
				security delete-certificate -Z $sha 2>/dev/null || true
			done
		`)
		cmd.Run()
		fmt.Printf("  ✓ CA removed from keychain\n")

		// Remove support directory
		os.RemoveAll(supportDir)
		fmt.Printf("  ✓ Support directory removed\n")
	} else {
		fmt.Printf("  ⏭  CA kept in keychain\n")
	}

	fmt.Println("\n===================")
	fmt.Println("Uninstall complete!")

	return nil
}
