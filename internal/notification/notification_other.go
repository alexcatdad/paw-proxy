//go:build !darwin && !linux

package notification

func notify(title, message string) error {
	return nil
}
