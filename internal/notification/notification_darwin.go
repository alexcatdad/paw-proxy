//go:build darwin

package notification

import (
	"fmt"
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
	safeTitle := sanitizeAppleScript(title)
	safeMessage := sanitizeAppleScript(message)

	script := fmt.Sprintf(`display notification "%s" with title "%s" sound name "Glass"`, safeMessage, safeTitle)
	return commandRunner("osascript", "-e", script).Run()
}
