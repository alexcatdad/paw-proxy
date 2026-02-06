// cmd/paw-proxy/main.go
package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/daemon"
	"github.com/alexcatdad/paw-proxy/internal/setup"
)

// version is set via -ldflags at build time; defaults to "dev" for local builds.
var version = "dev"

func main() {
	// Subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			cmdSetup()
			return
		case "uninstall":
			cmdUninstall()
			return
		case "status":
			cmdStatus()
			return
		case "run":
			cmdRun()
			return
		case "logs":
			cmdLogs()
			return
		case "version":
			fmt.Printf("paw-proxy version %s\n", version)
			return
		}
	}

	// Default: show usage
	fmt.Println("Usage: paw-proxy <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  setup      Configure DNS, CA, and install daemon (requires sudo)")
	fmt.Println("  uninstall  Remove all paw-proxy components")
	fmt.Println("  status     Show daemon status and registered routes")
	fmt.Println("  run        Run daemon in foreground (for launchd)")
	fmt.Println("  logs       Show daemon logs")
	fmt.Println("  version    Show version")
	os.Exit(1)
}

func cmdRun() {
	config, err := daemon.DefaultConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// SECURITY: Owner-only log file permissions
	logFile, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	// Write to both log file and stderr so startup failures are visible
	// when the daemon is run directly (e.g., CI, debugging).
	log.SetOutput(io.MultiWriter(logFile, os.Stderr))

	d, err := daemon.New(config)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	log.Println("paw-proxy daemon starting...")
	if err := d.Run(); err != nil {
		log.Fatalf("Daemon error: %v", err)
	}
}

func cmdSetup() {
	// Check for root/sudo
	if os.Geteuid() != 0 {
		fmt.Println("Error: setup requires sudo")
		fmt.Println("Run: sudo paw-proxy setup")
		os.Exit(1)
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Error: cannot determine binary path: %v\n", err)
		os.Exit(1)
	}

	defaultCfg, err := daemon.DefaultConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	config := &setup.Config{
		SupportDir: defaultCfg.SupportDir,
		BinaryPath: exe,
		DNSPort:    9353,
		TLD:        "test",
	}

	if err := setup.Run(config); err != nil {
		fmt.Printf("Setup failed: %v\n", err)
		os.Exit(1)
	}
}

func cmdUninstall() {
	brewFlag := false
	for _, arg := range os.Args[2:] {
		if arg == "--brew" {
			brewFlag = true
		}
	}

	config, err := daemon.DefaultConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if err := setup.Uninstall(config.SupportDir, "test", brewFlag); err != nil {
		fmt.Printf("Uninstall failed: %v\n", err)
		os.Exit(1)
	}
}

func cmdStatus() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	socketPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "paw-proxy.sock")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	// Check health
	resp, err := client.Get("http://unix/health")
	if err != nil {
		fmt.Println("Status: ❌ Daemon not running")
		fmt.Println("")
		fmt.Println("Run: sudo paw-proxy setup")
		return
	}
	defer resp.Body.Close()

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
	}
	json.NewDecoder(resp.Body).Decode(&health)

	fmt.Printf("Status: ✅ Running (v%s, up %s)\n", health.Version, health.Uptime)
	fmt.Println("")

	// Get routes
	resp, err = client.Get("http://unix/routes")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var routes []struct {
		Name          string    `json:"name"`
		Upstream      string    `json:"upstream"`
		Dir           string    `json:"dir"`
		Registered    time.Time `json:"registered"`
		LastHeartbeat time.Time `json:"lastHeartbeat"`
	}
	json.NewDecoder(resp.Body).Decode(&routes)

	if len(routes) == 0 {
		fmt.Println("Routes: (none)")
	} else {
		fmt.Println("Routes:")
		for _, r := range routes {
			age := time.Since(r.Registered).Round(time.Second)
			fmt.Printf("  • %s.test -> %s (%s)\n", r.Name, r.Upstream, age)
			fmt.Printf("    Dir: %s\n", r.Dir)
		}
	}

	// CA info
	certPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "ca.crt")
	if certData, err := os.ReadFile(certPath); err == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			cert, _ := x509.ParseCertificate(block.Bytes)
			if cert != nil {
				fmt.Println("")
				fmt.Printf("CA Expires: %s\n", cert.NotAfter.Format("2006-01-02"))
			}
		}
	}
}

func cmdLogs() {
	config, err := daemon.DefaultConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Parse flags
	tail := false
	clear := false
	for _, arg := range os.Args[2:] {
		switch arg {
		case "--tail", "-f":
			tail = true
		case "--clear":
			clear = true
		default:
			fmt.Printf("Unknown flag: %s\n", arg)
			fmt.Println("Usage: paw-proxy logs [--tail|-f] [--clear]")
			os.Exit(1)
		}
	}

	if clear {
		if err := os.Truncate(config.LogPath, 0); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No log file found")
				return
			}
			fmt.Printf("Error clearing logs: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Cleared %s\n", config.LogPath)
		return
	}

	fmt.Printf("Showing logs from %s\n", config.LogPath)
	fmt.Println(strings.Repeat("-", 50))

	if tail {
		cmdLogsTail(config.LogPath)
	} else {
		cmdLogsShow(config.LogPath, 50)
	}
}

// cmdLogsShow prints the last N lines of the log file.
func cmdLogsShow(path string, n int) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found -- daemon may not have run yet")
			return
		}
		fmt.Printf("Error reading log: %v\n", err)
		os.Exit(1)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	for _, line := range lines[start:] {
		fmt.Println(line)
	}
}

// cmdLogsTail follows the log file, printing new lines as they appear.
func cmdLogsTail(path string) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found -- daemon may not have run yet")
			return
		}
		fmt.Printf("Error opening log: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Seek to end
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		fmt.Printf("Error seeking: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Following log output (Ctrl+C to stop)...")

	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			// EOF is normal -- just wait for more data
			time.Sleep(200 * time.Millisecond)
			continue
		}
	}
}
