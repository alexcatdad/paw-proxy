//go:build !darwin && !linux

package help

func init() {
	// No platform-specific file paths on unsupported platforms.
}
