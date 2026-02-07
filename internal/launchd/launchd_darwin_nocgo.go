//go:build darwin && !cgo

// internal/launchd/launchd_darwin_nocgo.go
package launchd

import "net"

// ActivateSocket is a stub for builds without cgo (e.g., cross-compilation).
// Always returns (nil, false, nil) to trigger fallback to direct binding.
func ActivateSocket(_ string) (net.Listener, bool, error) {
	return nil, false, nil
}
