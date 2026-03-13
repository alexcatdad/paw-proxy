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
		expectedScriptPart := `display notification "test-message" with title "test-title"`
		if !strings.Contains(capturedArgs[1], expectedScriptPart) {
			t.Errorf("Script doesn't contain expected part: %q", capturedArgs[1])
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

func TestNotifyAppleScriptInjection(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("AppleScript injection tests only apply to darwin")
	}

	tests := []struct {
		name            string
		title           string
		message         string
		wantInScript    string
		notWantInScript string
	}{
		{
			name:         "double quotes in message",
			title:        "Alert",
			message:      `He said "hello"`,
			wantInScript: `display notification "He said \"hello\"" with title "Alert"`,
		},
		{
			name:         "double quotes in title",
			title:        `My "App"`,
			message:      "Something happened",
			wantInScript: `with title "My \"App\""`,
		},
		{
			name:         "backslashes in message",
			title:        "Alert",
			message:      `path\to\file`,
			wantInScript: `display notification "path\\to\\file"`,
		},
		{
			name:         "backslash before quote",
			title:        "Alert",
			message:      `test\"inject`,
			wantInScript: `display notification "test\\\"inject"`,
		},
		{
			name:         "AppleScript injection attempt has quotes escaped",
			title:        "Alert",
			message:      `" & do shell script "echo pwned" & "`,
			wantInScript: `display notification "\" & do shell script \"echo pwned\" & \""`,
		},
		{
			name:         "mixed backslashes and quotes",
			title:        `C:\Users\"admin"`,
			message:      `line1\nline2`,
			wantInScript: `with title "C:\\Users\\\"admin\""`,
		},
		{
			name:         "empty strings",
			title:        "",
			message:      "",
			wantInScript: `display notification "" with title ""`,
		},
		{
			name:         "normal usage is unchanged",
			title:        "paw-proxy",
			message:      "Project is live at: https://myapp.test",
			wantInScript: `display notification "Project is live at: https://myapp.test" with title "paw-proxy"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedArgs []string
			commandRunner = func(name string, arg ...string) *exec.Cmd {
				capturedArgs = arg
				return exec.Command("true")
			}

			err := Notify(tt.title, tt.message)
			if err != nil {
				t.Fatalf("Notify failed: %v", err)
			}

			if len(capturedArgs) < 2 {
				t.Fatal("Expected at least 2 args")
			}

			script := capturedArgs[1]

			if tt.wantInScript != "" && !strings.Contains(script, tt.wantInScript) {
				t.Errorf("Script should contain %q\n  got: %q", tt.wantInScript, script)
			}

			if tt.notWantInScript != "" && strings.Contains(script, tt.notWantInScript) {
				t.Errorf("Script should NOT contain %q\n  got: %q", tt.notWantInScript, script)
			}
		})
	}
}

func TestSanitizeAppleScript(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sanitizeAppleScript only available on darwin")
	}

	tests := []struct {
		input string
		want  string
	}{
		{`hello`, `hello`},
		{`say "hi"`, `say \"hi\"`},
		{`back\slash`, `back\\slash`},
		{`both\"combined`, `both\\\"combined`},
		{``, ``},
		{`no special chars here!`, `no special chars here!`},
		{`\\already escaped\\`, `\\\\already escaped\\\\`},
		{`"""`, `\"\"\"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeAppleScript(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeAppleScript(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
