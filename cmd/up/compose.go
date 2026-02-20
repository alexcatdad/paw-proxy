package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// composeDetection holds the result of scanning args for Docker Compose mode.
type composeDetection struct {
	detected     bool
	composeFlags []string // flags between "compose" and "up" (e.g., ["--profile", "frontend"])
	upIdx        int      // index of "up" in args
}

// detectDockerCompose checks if args represent a `docker compose ... up` command.
// Returns detection info including any compose-level flags captured between
// "compose" and "up" (e.g., --profile, -f, --project-name).
func detectDockerCompose(args []string) composeDetection {
	if len(args) < 3 || args[0] != "docker" || args[1] != "compose" {
		return composeDetection{}
	}

	for i := 2; i < len(args); i++ {
		if args[i] == "up" {
			var flags []string
			if i > 2 {
				flags = make([]string, i-2)
				copy(flags, args[2:i])
			}
			return composeDetection{
				detected:     true,
				composeFlags: flags,
				upIdx:        i,
			}
		}
	}

	return composeDetection{}
}

// composeConfig represents the JSON output of `docker compose config --format json`.
type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Ports []composePort `json:"ports"`
}

type composePort struct {
	Published string `json:"published"`
	Target    int    `json:"target"`
	Protocol  string `json:"protocol"`
}

// discoveredService represents a Docker Compose service with a published port.
type discoveredService struct {
	service       string
	publishedPort string
}

// composeRoute is a fully resolved route ready for registration.
type composeRoute struct {
	service   string // original compose service name
	routeName string // sanitized name for paw-proxy (e.g., "frontend.myapp")
	upstream  string // e.g., "localhost:3000"
}

// parseComposeConfig parses `docker compose config --format json` output and
// extracts services that have published ports.
func parseComposeConfig(data []byte) ([]discoveredService, string, error) {
	var config composeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, "", fmt.Errorf("parsing compose config: %w", err)
	}

	var services []discoveredService

	// Sort service names for deterministic output
	names := make([]string, 0, len(config.Services))
	for name := range config.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		svc := config.Services[name]
		if len(svc.Ports) == 0 {
			continue
		}
		port := svc.Ports[0]
		if port.Published == "" {
			continue
		}
		services = append(services, discoveredService{
			service:       name,
			publishedPort: port.Published,
		})
	}

	return services, config.Name, nil
}

// buildComposeRouteNames creates route entries with sanitized names.
// If nameFlag is set, it overrides the project name portion.
func buildComposeRouteNames(services []discoveredService, projectName, nameFlag string) []composeRoute {
	project := projectName
	if nameFlag != "" {
		project = nameFlag
	}
	project = sanitizeName(project)

	routes := make([]composeRoute, 0, len(services))
	for _, svc := range services {
		svcName := sanitizeName(svc.service)
		routes = append(routes, composeRoute{
			service:   svc.service,
			routeName: svcName + "." + project,
			upstream:  fmt.Sprintf("localhost:%s", svc.publishedPort),
		})
	}
	return routes
}

// multiRouteState manages multiple route entries for Docker Compose mode.
type multiRouteState struct {
	mu     sync.RWMutex
	routes []composeRoute
	dir    string
}

func newMultiRouteState(routes []composeRoute, dir string) *multiRouteState {
	return &multiRouteState{
		routes: routes,
		dir:    dir,
	}
}

// Snapshot returns a copy of all routes and the directory.
func (s *multiRouteState) Snapshot() ([]composeRoute, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	routes := make([]composeRoute, len(s.routes))
	copy(routes, s.routes)
	return routes, s.dir
}

// RouteNames returns the route names in order.
func (s *multiRouteState) RouteNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, len(s.routes))
	for i, r := range s.routes {
		names[i] = r.routeName
	}
	return names
}

// runComposeConfig runs `docker compose [flags] config --format json` and returns the raw output.
func runComposeConfig(composeFlags []string) ([]byte, error) {
	configArgs := []string{"compose"}
	configArgs = append(configArgs, composeFlags...)
	configArgs = append(configArgs, "config", "--format", "json")

	cmd := exec.Command("docker", configArgs...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose config failed: %w", err)
	}
	return output, nil
}

// registerComposeRoutes registers all compose routes with the daemon.
func registerComposeRoutes(client *http.Client, routes []composeRoute, dir string) error {
	for _, r := range routes {
		if err := registerRoute(client, r.routeName, r.upstream, dir); err != nil {
			return fmt.Errorf("registering %s: %w", r.routeName, err)
		}
	}
	return nil
}

// deregisterComposeRoutes deregisters all compose routes from the daemon.
func deregisterComposeRoutes(client *http.Client, routes []composeRoute) {
	for _, r := range routes {
		if err := deregisterRoute(client, r.routeName); err != nil {
			log.Printf("warning: deregister %s failed: %v", r.routeName, err)
		}
	}
}

// heartbeatCompose sends heartbeats for all compose routes.
func heartbeatCompose(ctx context.Context, client *http.Client, state *multiRouteState) {
	heartbeatComposeWithInterval(ctx, client, state, 10*time.Second)
}

func heartbeatComposeWithInterval(ctx context.Context, client *http.Client, state *multiRouteState, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			routes, dir := state.Snapshot()
			for _, r := range routes {
				req, err := http.NewRequest("POST", fmt.Sprintf("http://unix/routes/%s/heartbeat", r.routeName), nil)
				if err != nil {
					log.Printf("warning: compose heartbeat request failed for %s: %v", r.routeName, err)
					continue
				}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("warning: compose heartbeat failed for %s: %v", r.routeName, err)
					continue
				}

				if resp.StatusCode == http.StatusOK {
					resp.Body.Close()
					continue
				}
				resp.Body.Close()

				if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
					if err := registerRoute(client, r.routeName, r.upstream, dir); err != nil {
						log.Printf("warning: compose auto re-register failed for %s: %v", r.routeName, err)
						continue
					}
					log.Printf("route re-registered after daemon restart: %s.test -> %s", r.routeName, r.upstream)
					continue
				}

				log.Printf("warning: compose heartbeat for %s returned status %d", r.routeName, resp.StatusCode)
			}
		}
	}
}

// runDockerComposeMode handles the entire lifecycle when `up` wraps `docker compose up`.
func runDockerComposeMode(client *http.Client, dc composeDetection, args []string, caPath string) {
	// 1. Discover services via docker compose config
	configOutput, err := runComposeConfig(dc.composeFlags)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	services, projectName, err := parseComposeConfig(configOutput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(services) == 0 {
		fmt.Println("Error: no services with published ports found in docker-compose config")
		os.Exit(1)
	}

	// 2. Build route names
	routes := buildComposeRouteNames(services, projectName, *nameFlag)

	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	state := newMultiRouteState(routes, dir)

	// 3. Register all routes
	if err := registerComposeRoutes(client, routes, dir); err != nil {
		fmt.Printf("Error registering routes: %v\n", err)
		os.Exit(1)
	}

	// 4. Print route mappings
	for _, r := range routes {
		fmt.Printf("Mapping https://%s.test -> %s...\n", r.routeName, r.upstream)
	}
	fmt.Printf("%d services live:\n", len(routes))
	for _, r := range routes {
		fmt.Printf("   https://%s.test\n", r.routeName)
	}
	fmt.Println("------------------------------------------------")

	// 5. Start heartbeat
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeatCompose(ctx, client, state)

	// 6. Cleanup function
	cleanup := func() {
		fmt.Printf("\nRemoving %d route mappings...\n", len(routes))
		deregisterComposeRoutes(client, routes)
	}

	// 7. Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 8. Run docker compose up as child process
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NODE_EXTRA_CA_CERTS=%s", caPath),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting docker compose: %v\n", err)
		cancel()
		cleanup()
		os.Exit(1)
	}

	// 9. Wait for signal or command exit
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	var exitCode int
	select {
	case sig := <-sigCh:
		syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		select {
		case <-doneCh:
		case <-time.After(10 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
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
