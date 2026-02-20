//go:build linux

package main

import "os"

// doctorCheckDNS verifies the systemd-resolved stub zone config exists.
func doctorCheckDNS() (bool, string) {
	confPath := "/etc/systemd/resolved.conf.d/paw-proxy.conf"
	if _, err := os.Stat(confPath); err != nil {
		return false, "systemd-resolved config missing (/etc/systemd/resolved.conf.d/paw-proxy.conf)"
	}
	return true, "systemd-resolved stub zone configured"
}
