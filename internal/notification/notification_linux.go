//go:build linux

package notification

func notify(title, message string) error {
	// Simple notify-send implementation
	return commandRunner("notify-send", title, message).Run()
}
