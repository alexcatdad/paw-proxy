//go:build !darwin && !linux

package paths

import "fmt"

// DefaultPaths returns an error on unsupported platforms.
func DefaultPaths() (*Paths, error) {
	return nil, fmt.Errorf("paw-proxy: unsupported platform")
}
