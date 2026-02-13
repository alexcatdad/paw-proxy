// cmd/up/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/help"
)

// version is set via -ldflags at build time; defaults to "dev" for local builds.
var version = "dev"

var (
	nameFlag    = flag.String("n", "", "Custom app name (default: from package.json or directory)")
	restartFlag = flag.Bool("restart", false, "Auto-restart on crash (non-zero exit)")
	showVersion = flag.Bool("version", false, "Show version")
	showVersionShort = flag.Bool("v", false, "")
)

type routeState struct {
	mu       sync.RWMutex
	name     string
	upstream string
	dir      string
}

func newRouteState(name, dir string) *routeState {
	return &routeState{name: name, dir: dir}
}

func (s *routeState) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

func (s *routeState) SetUpstream(upstream string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upstream = upstream
}

func (s *routeState) Snapshot() (name string, upstream string, dir string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.name, s.upstream, s.dir
}

func main() {
	help.UpCommand.Version = version

	flag.Usage = func() {
		help.UpCommand.Render(os.Stderr)
	}
	flag.Parse()

	// Handle --version/-v flag
	if *showVersion || *showVersionShort {
		fmt.Printf("up version %s\n", version)
		return
	}

	if flag.NArg() == 0 {
		help.UpCommand.Render(os.Stderr)
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

	// Check if daemon is running via health endpoint
	client := socketClient(socketPath)
	{
		resp, err := client.Get("http://unix/health")
		if err != nil {
			fmt.Println("Error: paw-proxy daemon not running")
			fmt.Println("Run: sudo paw-proxy setup")
			os.Exit(1)
		}
		resp.Body.Close()
	}

	// Determine app name
	name := determineName(*nameFlag)
	dir, _ := os.Getwd()
	state := newRouteState(name, dir)

	// Setup cleanup (deregisters route from daemon)
	cleanup := func() {
		fmt.Printf("\n🛑 Removing mapping for %s.test...\n", name)
		if err := deregisterRoute(client, name); err != nil {
			log.Printf("warning: cleanup deregistration failed: %v", err)
		}
	}

	// Start heartbeat (runs for the entire lifetime, across restarts)
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeat(ctx, client, state)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	var exitCode int
	for {
		// Find free port
		port, err := findFreePort()
		if err != nil {
			fmt.Printf("Error finding free port: %v\n", err)
			os.Exit(1)
		}

		upstream := fmt.Sprintf("localhost:%d", port)
		state.SetUpstream(upstream)

		// On restart, deregister old route first so re-registration succeeds
		if exitCode != 0 {
			if err := deregisterRoute(client, name); err != nil {
				log.Printf("warning: restart deregistration failed: %v", err)
			}
		}

		// Register route
		err = registerRoute(client, name, upstream, dir)
		if err != nil {
			if conflictDir := extractConflictDir(err); conflictDir != "" {
				dirName := sanitizeName(filepath.Base(dir))
				if dirName != name {
					fmt.Printf("⚠️  %s.test already in use from %s\n", name, conflictDir)
					fmt.Printf("   Using %s.test instead\n", dirName)
					name = dirName
					state.SetName(name)
					err = registerRoute(client, name, upstream, dir)
				}
			}
			if err != nil {
				fmt.Printf("Error registering route: %v\n", err)
				os.Exit(1)
			}
		}

		fmt.Printf("🔗 Mapping https://%s.test -> localhost:%d...\n", name, port)
		if exitCode == 0 {
			fmt.Printf("🚀 Project is live at: https://%s.test\n", name)
		} else {
			fmt.Printf("🔄 Restarting (previous exit code: %d)...\n", exitCode)
		}
		fmt.Println("------------------------------------------------")

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

		// Run child in its own process group so we can signal the entire group
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if err := cmd.Start(); err != nil {
			fmt.Printf("Error starting command: %v\n", err)
			break
		}

		// Wait for signal or command exit
		doneCh := make(chan error, 1)
		go func() {
			doneCh <- cmd.Wait()
		}()

		gotSignal := false
		select {
		case sig := <-sigCh:
			gotSignal = true
			// Forward signal to entire process group (negative PID)
			syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
			// Wait for child with timeout
			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case err := <-doneCh:
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 0
			}
		}

		if gotSignal {
			break
		}

		// If not restarting, or clean exit, stop the loop
		if !*restartFlag || exitCode == 0 {
			break
		}

		fmt.Printf("\n⚠️  Process exited with code %d, restarting in 1s...\n", exitCode)

		// Brief delay before restart to avoid tight crash loops
		select {
		case <-time.After(1 * time.Second):
		case <-sigCh:
			goto done
		}
	}

done:

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
		return sanitizeName(explicit)
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
	name = strings.ToLower(name)
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else {
			result = append(result, '-')
		}
	}
	s := strings.Trim(string(result), "-")
	if s == "" {
		return "app"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "app-" + s
	}
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
		if s == "" {
			return "app"
		}
	}
	return s
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

// conflictError represents a route name conflict from the daemon API.
type conflictError struct {
	dir string
}

func (e *conflictError) Error() string {
	return fmt.Sprintf("route conflict: already registered from %s", e.dir)
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

	if resp.StatusCode == http.StatusConflict {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return &conflictError{dir: errResp["existingDir"]}
	}

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("%s: %s", resp.Status, errResp["error"])
	}

	return nil
}

func deregisterRoute(client *http.Client, name string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://unix/routes/%s", name), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected deregister status: %s", resp.Status)
	}

	return nil
}

func heartbeat(ctx context.Context, client *http.Client, state *routeState) {
	heartbeatWithInterval(ctx, client, state, 10*time.Second)
}

func heartbeatWithInterval(ctx context.Context, client *http.Client, state *routeState, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			name, _, _ := state.Snapshot()
			req, err := http.NewRequest("POST", fmt.Sprintf("http://unix/routes/%s/heartbeat", name), nil)
			if err != nil {
				log.Printf("warning: heartbeat request creation failed: %v", err)
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("warning: heartbeat failed: %v", err)
				continue
			}

			if resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
				name, upstream, dir := state.Snapshot()
				if upstream == "" {
					log.Printf("warning: heartbeat route missing but no upstream available for %s", name)
					continue
				}
				if err := registerRoute(client, name, upstream, dir); err != nil {
					log.Printf("warning: auto re-register failed: %v", err)
					continue
				}
				log.Printf("route re-registered after daemon restart: %s.test -> %s", name, upstream)
				continue
			}

			log.Printf("warning: heartbeat returned status %s", resp.Status)
		}
	}
}

func extractConflictDir(err error) string {
	var ce *conflictError
	if errors.As(err, &ce) {
		return ce.dir
	}
	return ""
}
