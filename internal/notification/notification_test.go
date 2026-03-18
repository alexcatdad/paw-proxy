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
		// May use terminal-notifier or osascript depending on what's installed.
		// Either way, verify a command was invoked with the title and message.
		if capturedCmd == "" {
			t.Fatal("Expected a command to be invoked on darwin")
		}
		fullArgs := strings.Join(capturedArgs, " ")
		if !strings.Contains(fullArgs, title) || !strings.Contains(fullArgs, message) {
			t.Errorf("Args should contain title and message: %v", capturedArgs)
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
