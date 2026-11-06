package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/ssl"
)

func TestNewRequiresCA(t *testing.T) {
	dir := t.TempDir()
	_, err := New(&Config{SupportDir: dir, LogPath: filepath.Join(dir, "log", "d.log"), TLD: "test"})
	if err == nil || !strings.Contains(err.Error(), "CA not found") {
		t.Fatalf("New without a CA: got %v, want a 'CA not found' error", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	c, err := DefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.TLD != "test" || c.HTTPPort != 80 || c.HTTPSPort != 443 || c.DNSPort != 9353 {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.SocketPath == "" || c.LogPath == "" || c.SupportDir == "" {
		t.Errorf("paths not set: %+v", c)
	}
}

// TestRunStartsAndShutsDown runs the whole daemon on ephemeral loopback
// ports and stops it with SIGTERM, which Run handles itself.
func TestRunStartsAndShutsDown(t *testing.T) {
	dir := t.TempDir()
	if err := ssl.GenerateCA(filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")); err != nil {
		t.Fatal(err)
	}
	config := &Config{
		DNSPort:    0,
		HTTPPort:   0,
		HTTPSPort:  0,
		TLD:        "test",
		SupportDir: dir,
		SocketPath: filepath.Join(dir, "d.sock"),
		LogPath:    filepath.Join(dir, "logs", "daemon.log"),
	}
	d, err := New(config)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- d.Run() }()

	// Run installs its signal handler before starting the API server, so
	// once the socket answers, SIGTERM goes to Run, not to the test binary.
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.Dial("unix", config.SocketPath)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("API socket never came up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not shut down after SIGTERM")
	}

	if _, err := os.Stat(config.SocketPath); !os.IsNotExist(err) {
		t.Errorf("socket not removed after shutdown: %v", err)
	}
	logData, err := os.ReadFile(config.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "shutdown complete") {
		t.Errorf("log does not record shutdown:\n%s", logData)
	}
}
