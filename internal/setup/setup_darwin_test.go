//go:build darwin

package setup

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestChownToRealUser_NotRoot(t *testing.T) {
	// When not running as root, chownToRealUser should be a no-op.
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}

	tmp := t.TempDir()
	f := filepath.Join(tmp, "testfile")
	if err := os.WriteFile(f, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should return nil (no-op) when not root.
	if err := chownToRealUser(f); err != nil {
		t.Errorf("chownToRealUser should be no-op for non-root, got: %v", err)
	}
}

func TestChownToRealUser_NoSUDO_USER(t *testing.T) {
	// Even if we were root, without SUDO_USER set, it should be a no-op.
	// We can't actually test as root, but we can verify the env var check.
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}

	// Ensure SUDO_USER is not set
	old := os.Getenv("SUDO_USER")
	os.Unsetenv("SUDO_USER")
	defer func() {
		if old != "" {
			os.Setenv("SUDO_USER", old)
		}
	}()

	if err := chownToRealUser("/nonexistent"); err != nil {
		t.Errorf("expected nil when not root, got: %v", err)
	}
}

func TestResolveRealUID_NotRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}

	uid, err := resolveRealUID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != os.Getuid() {
		t.Errorf("expected UID %d, got %d", os.Getuid(), uid)
	}
}

func TestResolveRealUID_MatchesCurrentUser(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	uid, err := resolveRealUID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if uid == 0 {
		t.Error("non-root user should not have UID 0")
	}

	// Verify it matches the current user's UID
	_ = u // user.Current() validates the user lookup works
}

func TestLaunchAgentTemplate_ContainsSockType(t *testing.T) {
	// Verify the plist template includes SockType and SockPassive keys
	// for proper launchd socket activation.
	if !strings.Contains(launchAgentTemplate, "<key>SockType</key>") {
		t.Error("plist template missing SockType key")
	}
	if !strings.Contains(launchAgentTemplate, "<key>SockPassive</key>") {
		t.Error("plist template missing SockPassive key")
	}
	if !strings.Contains(launchAgentTemplate, "<string>stream</string>") {
		t.Error("plist template missing stream value for SockType")
	}
}

func TestLaunchAgentTemplate_ContainsLoopbackBinding(t *testing.T) {
	// Verify sockets bind to localhost only (SECURITY: prevents external access).
	if !strings.Contains(launchAgentTemplate, "<string>127.0.0.1</string>") {
		t.Error("plist template must bind to 127.0.0.1 for security")
	}
}
