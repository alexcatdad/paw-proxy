package notification

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestNotify(t *testing.T) {
	var capturedCmd string
	var capturedArgs []string

	// Mock command runner
	commandRunner = func(name string, arg ...string) *exec.Cmd {
		capturedCmd = name
		capturedArgs = arg
		// Return a command that does nothing but exists
		return exec.Command("true")
	}

	title := "test-title"
	message := "test-message"
	err := Notify(title, message)

	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	switch runtime.GOOS {
	case "darwin":
		if capturedCmd != "osascript" {
			t.Errorf("Expected command 'osascript', got %q", capturedCmd)
		}
		if len(capturedArgs) != 2 || capturedArgs[0] != "-e" {
			t.Errorf("Unexpected args: %v", capturedArgs)
		}
		if !strings.Contains(capturedArgs[1], title) || !strings.Contains(capturedArgs[1], message) {
			t.Errorf("Script should contain title and message: %q", capturedArgs[1])
		}
	case "linux":
		if capturedCmd != "notify-send" {
			t.Errorf("Expected command 'notify-send', got %q", capturedCmd)
		}
		if len(capturedArgs) != 2 || capturedArgs[0] != title || capturedArgs[1] != message {
			t.Errorf("Unexpected args: %v", capturedArgs)
		}
	default:
		// Other platforms should be no-ops and not call commandRunner
		if capturedCmd != "" {
			t.Errorf("Expected no command for %s, got %q", runtime.GOOS, capturedCmd)
		}
	}
}
