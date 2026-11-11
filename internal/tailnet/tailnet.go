// Package tailnet exposes a route's loopback port to the user's tailnet
// through the Tailscale CLI (`tailscale serve`). It never enables Funnel and
// only removes Serve routes it created and that are still unchanged.
package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// appCLI is where Tailscale.app keeps its CLI when `tailscale` is not on PATH.
const appCLI = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"

// Port returns the tailnet HTTPS port for a route name. It is derived from
// the name, so a route keeps the same tailnet URL across restarts.
func Port(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return 20000 + int(h.Sum32()%10000)
}

// Exposer manages the Serve routes this daemon created. The ledger file
// records them so a later start can remove what an unclean exit left behind.
type Exposer struct {
	run    func(ctx context.Context, args ...string) ([]byte, error)
	alive  func(target string) bool
	ledger string

	mu     sync.Mutex
	served map[string]string // tailnet port -> proxy target
}

// New returns an Exposer that keeps its ledger at ledgerPath.
func New(ledgerPath string) *Exposer {
	e := &Exposer{run: runCLI, alive: listening, ledger: ledgerPath, served: map[string]string{}}
	if data, err := os.ReadFile(ledgerPath); err == nil { //nolint:gosec // G304: path comes from internal/paths
		_ = json.Unmarshal(data, &e.served)
	}
	return e
}

func runCLI(ctx context.Context, args ...string) ([]byte, error) {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		if _, statErr := os.Stat(appCLI); statErr != nil {
			return nil, errors.New("tailscale CLI not found")
		}
		bin = appCLI
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: fixed binary; arguments are flags and validated ports
	// Without a terminal (the daemon runs under launchd) Tailscale.app's
	// binary starts as the GUI unless told to be the CLI.
	cmd.Env = append(os.Environ(), "TAILSCALE_BE_CLI=1")
	return cmd.Output()
}

// listening reports whether a proxy target such as http://127.0.0.1:3000
// has a listener.
func listening(target string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return true // unknown target: treat as in use
	}
	conn, err := net.DialTimeout("tcp", u.Host, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

type serveConfig struct {
	TCP map[string]json.RawMessage
	Web map[string]struct {
		Handlers map[string]struct{ Proxy string }
	}
	AllowFunnel map[string]bool
}

// node returns this machine's tailnet DNS name, or why it cannot serve HTTPS.
func (e *Exposer) node(ctx context.Context) (string, error) {
	out, err := e.run(ctx, "status", "--json")
	if err != nil {
		return "", fmt.Errorf("tailscale is not available: %w", err)
	}
	var st struct {
		BackendState string
		Self         struct{ DNSName string }
		CertDomains  []string
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return "", errors.New("tailscale status could not be read")
	}
	host := strings.TrimSuffix(st.Self.DNSName, ".")
	if st.BackendState != "Running" || host == "" {
		return "", errors.New("tailscale is not running")
	}
	if !slices.Contains(st.CertDomains, host) {
		return "", errors.New("tailnet HTTPS certificates are not enabled")
	}
	return host, nil
}

func (e *Exposer) config(ctx context.Context) (serveConfig, error) {
	var cfg serveConfig
	out, err := e.run(ctx, "serve", "status", "--json")
	if err != nil {
		return cfg, fmt.Errorf("tailscale serve is not available: %w", err)
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return cfg, errors.New("tailscale serve configuration could not be read")
	}
	return cfg, nil
}

// Expose serves upstream (a loopback host:port) on the tailnet and returns
// its URL, https://<machine>.<tailnet>.ts.net:<port>.
func (e *Exposer) Expose(ctx context.Context, name, upstream string) (string, error) {
	_, upstreamPort, err := net.SplitHostPort(upstream)
	if err != nil {
		return "", errors.New("invalid upstream")
	}
	target := "http://127.0.0.1:" + upstreamPort
	port := strconv.Itoa(Port(name))

	e.mu.Lock()
	defer e.mu.Unlock()

	host, err := e.node(ctx)
	if err != nil {
		return "", err
	}
	cfg, err := e.config(ctx)
	if err != nil {
		return "", err
	}
	key := net.JoinHostPort(host, port)
	if cfg.AllowFunnel[key] {
		return "", fmt.Errorf("funnel is enabled on tailnet port %s", port)
	}
	web, hasWeb := cfg.Web[key]
	if _, hasTCP := cfg.TCP[port]; hasTCP && !hasWeb {
		return "", fmt.Errorf("tailnet port %s is in use", port)
	}
	for _, h := range web.Handlers {
		// A handler we do not recognise, or one whose target still answers,
		// belongs to something else. A dead proxy target is a leftover.
		if h.Proxy == "" || (h.Proxy != target && e.alive(h.Proxy)) {
			return "", fmt.Errorf("tailnet port %s is in use", port)
		}
	}
	if _, err := e.run(ctx, "serve", "--bg", "--yes", "--https="+port, target); err != nil {
		return "", fmt.Errorf("tailscale serve failed: %w", err)
	}
	e.served[port] = target
	e.save()
	return "https://" + key, nil
}

// Unexpose removes the Serve route created for name, if this daemon created
// it and it still points at the same target.
func (e *Exposer) Unexpose(ctx context.Context, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.remove(ctx, strconv.Itoa(Port(name)))
	e.save()
}

// Reset removes every Serve route in the ledger: leftovers at daemon start,
// live routes at shutdown.
func (e *Exposer) Reset(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.served) == 0 {
		return
	}
	for port := range e.served {
		e.remove(ctx, port)
	}
	e.save()
}

// remove turns one port off. On a CLI failure the entry stays in the ledger
// so the next Reset retries it.
func (e *Exposer) remove(ctx context.Context, port string) {
	target, ok := e.served[port]
	if !ok {
		return
	}
	host, err := e.node(ctx)
	if err != nil {
		return
	}
	cfg, err := e.config(ctx)
	if err != nil {
		return
	}
	handlers := cfg.Web[net.JoinHostPort(host, port)].Handlers
	if len(handlers) == 1 && handlers["/"].Proxy == target {
		if _, err := e.run(ctx, "serve", "--yes", "--https="+port, "off"); err != nil {
			return
		}
	}
	delete(e.served, port)
}

func (e *Exposer) save() {
	data, err := json.Marshal(e.served)
	if err != nil {
		return
	}
	tmp := e.ledger + ".tmp"
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Rename(tmp, e.ledger)
	}
}
