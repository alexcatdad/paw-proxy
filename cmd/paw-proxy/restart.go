package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/daemon"
)

// errNoService means no paw-proxy service is installed for this user.
var errNoService = errors.New("no paw-proxy service is installed; run: sudo paw-proxy setup")

type healthInfo struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Uptime  string `json:"uptime"`
}

func cmdRestart() {
	if os.Getuid() == 0 {
		fmt.Println("Run 'paw-proxy restart' as your user, without sudo.")
		os.Exit(1)
	}
	if err := restartService(); err != nil {
		fmt.Printf("Restart failed: %v\n", err)
		os.Exit(1)
	}

	config, err := daemon.DefaultConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	h, err := waitForHealth(func() (healthInfo, error) { return fetchHealth(config.SocketPath) }, 40, 250*time.Millisecond)
	if err != nil {
		fmt.Printf("Restarted, but the daemon is not answering yet: %v\n", err)
		fmt.Println("Check: paw-proxy logs")
		os.Exit(1)
	}
	fmt.Printf("Restarted: %s, up %s\n", describeVersion(h), h.Uptime)
}

func describeVersion(h healthInfo) string {
	if h.Build == "" {
		return "v" + h.Version
	}
	return fmt.Sprintf("v%s (build %s)", h.Version, h.Build)
}

// waitForHealth polls get until it reports status "ok" or tries run out.
func waitForHealth(get func() (healthInfo, error), tries int, interval time.Duration) (healthInfo, error) {
	var lastErr error
	for i := 0; i < tries; i++ {
		if i > 0 {
			time.Sleep(interval)
		}
		h, err := get()
		if err == nil && h.Status == "ok" {
			return h, nil
		}
		if err == nil {
			err = fmt.Errorf("status %q", h.Status)
		}
		lastErr = err
	}
	return healthInfo{}, lastErr
}

func fetchHealth(socketPath string) (healthInfo, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: time.Second,
	}
	resp, err := client.Get("http://unix/health")
	if err != nil {
		return healthInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var h healthInfo
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return healthInfo{}, err
	}
	return h, nil
}
