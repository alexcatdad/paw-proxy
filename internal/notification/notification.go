package notification

import (
	"os/exec"
)

// commandRunner is used to execute shell commands.
// It can be replaced in tests to verify command arguments.
var commandRunner func(name string, arg ...string) *exec.Cmd = exec.Command

// Notify sends a desktop notification.
func Notify(title, message string) error {
	return notify(title, message)
}
