//go:build darwin

package notification

import (
	"fmt"
	"os/exec"
	"strings"
)

// sanitizeAppleScript makes a string safe for embedding inside an AppleScript
// double-quoted string literal. AppleScript treats backslash as an escape
// character and double-quote terminates the string, so both must be escaped.
// We escape backslashes first (so we don't double-escape the ones we add for
// quotes), then escape double quotes.
func sanitizeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func notify(title, message string) error {
	// Prefer terminal-notifier: it doesn't open Script Editor and supports
	// custom icons via the notification sender identity.
	if path, err := exec.LookPath("terminal-notifier"); err == nil {
		return commandRunner(path,
			"-title", title,
			"-message", message,
			"-sound", "Glass",
			"-appIcon", "https://raw.githubusercontent.com/paw-proxy/paw-proxy/main/docs/icon.png",
		).Run()
	}

	// Fallback: use osascript but route through System Events so that macOS
	// attributes the notification to System Events instead of Script Editor.
	// This prevents Script Editor from opening and bouncing in the Dock.
	safeTitle := sanitizeAppleScript(title)
	safeMessage := sanitizeAppleScript(message)
	script := fmt.Sprintf(
		`tell application "System Events" to display notification "%s" with title "%s" sound name "Glass"`,
		safeMessage, safeTitle,
	)
	return commandRunner("osascript", "-e", script).Run()
}
