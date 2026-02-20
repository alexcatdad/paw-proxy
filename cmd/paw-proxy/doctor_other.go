//go:build !darwin && !linux

package main

// doctorCheckDNS is a stub for unsupported platforms.
func doctorCheckDNS() (bool, string) {
	return false, "DNS check not supported on this platform"
}
