//go:build !darwin

// internal/launchd/launchd_other.go
package launchd

import "net"

// ActivateSocket is a stub for non-macOS platforms.
// Always returns (nil, false, nil) to trigger fallback to direct binding.
func ActivateSocket(_ string) (net.Listener, bool, error) {
	return nil, false, nil
}
