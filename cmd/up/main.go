// cmd/up/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var (
	nameFlag = flag.String("n", "", "Custom app name (default: from package.json or directory)")
)

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println("Usage: up [-n name] <command> [args...]")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -n name    Custom domain name (default: package.json name or directory)")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  up bun dev")
		fmt.Println("  up -n myapp npm run dev")
		os.Exit(1)
	}

	// Get socket path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	socketPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "paw-proxy.sock")
	caPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "ca.crt")

	// Check if daemon is running
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		fmt.Println("Error: paw-proxy daemon not running")
		fmt.Println("Run: sudo paw-proxy setup")
		os.Exit(1)
	}

	// Find free port
	port, err := findFreePort()
	if err != nil {
		fmt.Printf("Error finding free port: %v\n", err)
		os.Exit(1)
	}

	// Determine app name
	name := determineName(*nameFlag)
	dir, _ := os.Getwd()

	// Register route
	client := socketClient(socketPath)
	err = registerRoute(client, name, fmt.Sprintf("localhost:%d", port), dir)
	if err != nil {
		// Check for conflict
		if conflictDir := extractConflictDir(err); conflictDir != "" {
			// Try directory name fallback
			dirName := filepath.Base(dir)
			if dirName != name {
				fmt.Printf("⚠️  %s.test already in use from %s\n", name, conflictDir)
				fmt.Printf("   Using %s.test instead\n", dirName)
				name = dirName
				err = registerRoute(client, name, fmt.Sprintf("localhost:%d", port), dir)
			}
		}
		if err != nil {
			fmt.Printf("Error registering route: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("🔗 Mapping https://%s.test -> localhost:%d...\n", name, port)
	fmt.Printf("🚀 Project is live at: https://%s.test\n", name)
	fmt.Println("------------------------------------------------")

	// Setup cleanup
	cleanup := func() {
		fmt.Printf("\n🛑 Removing mapping for %s.test...\n", name)
		deregisterRoute(client, name)
	}

	// Start heartbeat
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeat(ctx, client, name)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Build command
	args := flag.Args()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("APP_DOMAIN=%s.test", name),
		fmt.Sprintf("APP_URL=https://%s.test", name),
		"HTTPS=true",
		fmt.Sprintf("NODE_EXTRA_CA_CERTS=%s", caPath),
	)

	// Start command
	if err := cmd.Start(); err != nil {
		cleanup()
		fmt.Printf("Error starting command: %v\n", err)
		os.Exit(1)
	}

	// Wait for signal or command exit
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	var exitCode int
	select {
	case sig := <-sigCh:
		// Forward signal to child
		cmd.Process.Signal(sig)
		// Wait for child with timeout
		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	case err := <-doneCh:
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	cancel()
	cleanup()
	os.Exit(exitCode)
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func determineName(explicit string) string {
	if explicit != "" {
		return explicit
	}

	// Try package.json
	if data, err := os.ReadFile("package.json"); err == nil {
		var pkg struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Name != "" {
			return sanitizeName(pkg.Name)
		}
	}

	// Fall back to directory name
	dir, _ := os.Getwd()
	return sanitizeName(filepath.Base(dir))
}

func sanitizeName(name string) string {
	// Replace non-alphanumeric with dashes
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

func socketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func registerRoute(client *http.Client, name, upstream, dir string) error {
	body, _ := json.Marshal(map[string]string{
		"name":     name,
		"upstream": upstream,
		"dir":      dir,
	})

	resp, err := client.Post("http://unix/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("%s: %s", resp.Status, errResp["error"])
	}

	return nil
}

func deregisterRoute(client *http.Client, name string) {
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://unix/routes/%s", name), nil)
	client.Do(req)
}

func heartbeat(ctx context.Context, client *http.Client, name string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, _ := http.NewRequest("POST", fmt.Sprintf("http://unix/routes/%s/heartbeat", name), nil)
			client.Do(req)
		}
	}
}

func extractConflictDir(err error) string {
	// Parse error message for conflict info
	// This is a simplification - real impl would parse JSON response
	return ""
}
