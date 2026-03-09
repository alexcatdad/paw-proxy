//go:build darwin

package notification

import (
	"fmt"
	"strings"
)

func notify(title, message string) error {
	// Simple osascript implementation
	// We escape double quotes to avoid script errors
	escapedMessage := strings.ReplaceAll(message, "\"", "\\\"")
	escapedTitle := strings.ReplaceAll(title, "\"", "\\\"")

	script := fmt.Sprintf("display notification %q with title %q sound name %q", escapedMessage, escapedTitle, "Glass")
	return commandRunner("osascript", "-e", script).Run()
}
