//go:build darwin

package main

import "os"

// doctorCheckDNS verifies the macOS DNS resolver file exists.
func doctorCheckDNS() (bool, string) {
	resolverPath := "/etc/resolver/test"
	if _, err := os.Stat(resolverPath); err != nil {
		return false, "DNS resolver missing (/etc/resolver/test)"
	}
	return true, "DNS resolver configured (/etc/resolver/test)"
}
