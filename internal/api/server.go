// internal/api/server.go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

// Version is set via -ldflags at build time; defaults to "dev" for local builds.
var Version = "dev"

// Build is the bundle build number (CFBundleVersion) when the binary ships
// inside the macOS app; empty for standalone builds. Set via -ldflags.
var Build = ""

// tailnetTimeout bounds the Tailscale CLI calls made for one request, so a
// stuck Tailscale never holds a registration open.
const tailnetTimeout = 6 * time.Second

// Tailnet serves routes on the user's tailnet. The daemon supplies
// internal/tailnet; nil means tailnet serving is unavailable.
type Tailnet interface {
	Expose(ctx context.Context, name, upstream string) (url string, err error)
	Unexpose(ctx context.Context, name string)
}

// Max request body size (1MB)
const maxRequestBodySize = 1024 * 1024

// Route name validation pattern: starts with letter or digit; rest can be alphanumeric, dash, underscore, or dot.
var routeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

type Server struct {
	socketPath string
	registry   *RouteRegistry
	server     *http.Server
	listener   net.Listener
	startTime  time.Time

	notifications *NotificationQueue
	tailnet       Tailnet
}

// SetTailnet enables `tailnet: true` registrations. Call it before Start.
func (s *Server) SetTailnet(t Tailnet) { s.tailnet = t }

func NewServer(socketPath string, registry *RouteRegistry) *Server {
	s := &Server{
		socketPath: socketPath,
		registry:   registry,
		startTime:  time.Now(),

		notifications: NewNotificationQueue(notificationQueueSize),
	}

	// SECURITY: Per-endpoint rate limiters prevent runaway scripts from causing
	// unbounded route map growth or CPU-intensive cert generation.
	routeRegLimiter := newRateLimiter(10)
	heartbeatLimiter := newRateLimiter(100)
	routeDeleteLimiter := newRateLimiter(10)
	routeListLimiter := newRateLimiter(50)
	healthLimiter := newRateLimiter(100)
	notifyLimiter := newRateLimiter(10)
	notificationListLimiter := newRateLimiter(50)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /routes", rateLimit(routeRegLimiter, s.handleRegister))
	mux.HandleFunc("DELETE /routes/{name}", rateLimit(routeDeleteLimiter, s.handleDeregister))
	mux.HandleFunc("POST /routes/{name}/heartbeat", rateLimit(heartbeatLimiter, s.handleHeartbeat))
	mux.HandleFunc("GET /routes", rateLimit(routeListLimiter, s.handleList))
	mux.HandleFunc("GET /health", rateLimit(healthLimiter, s.handleHealth))
	mux.HandleFunc("POST /notifications", rateLimit(notifyLimiter, s.handleNotify))
	mux.HandleFunc("GET /notifications", rateLimit(notificationListLimiter, s.handleNotifications))

	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	return s
}

func (s *Server) Start() error {
	// Remove existing socket
	_ = os.Remove(s.socketPath)

	// SECURITY: Set umask before creating socket so it is born with 0600
	// permissions. This avoids the TOCTOU race between Listen and Chmod
	// where another process could connect during the gap.
	oldMask := syscall.Umask(0077)
	var err error
	s.listener, err = net.Listen("unix", s.socketPath)
	syscall.Umask(oldMask)
	if err != nil {
		return err
	}

	return s.server.Serve(s.listener)
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

type RegisterRequest struct {
	Name     string `json:"name"`
	Upstream string `json:"upstream"`
	Dir      string `json:"dir"`
	// Tailnet asks the daemon to also serve the route on the tailnet.
	Tailnet bool `json:"tailnet,omitempty"`
}

// RegisterResponse is the body of a successful registration that asked for
// tailnet serving. The route is registered either way: TailnetError says why
// it is reachable at its .test name only.
type RegisterResponse struct {
	TailnetURL   string `json:"tailnetURL,omitempty"`
	TailnetError string `json:"tailnetError,omitempty"`
}

// validateRouteName ensures route names are safe for DNS, filesystem, and shell use
func validateRouteName(name string) error {
	if !routeNamePattern.MatchString(name) {
		return fmt.Errorf("invalid route name: must start with a letter or digit and contain only letters, numbers, dashes, underscores, or dots (max 63 chars)")
	}
	return nil
}

// validateUpstream ensures upstream targets are localhost only (prevent SSRF)
func validateUpstream(upstream string) error {
	host, portStr, err := net.SplitHostPort(upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream format: expected host:port")
	}

	// SECURITY: Only allow localhost/loopback to prevent SSRF.
	// Use net.ParseIP to correctly handle all loopback representations
	// including IPv6 (::1) and IPv4 (127.0.0.1).
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("upstream must be localhost or loopback address")
	}

	// Validate port is numeric and in valid range
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: must be 1-65535")
	}

	return nil
}

// validateDir ensures directory paths are absolute and don't contain traversal
func validateDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("directory is required")
	}

	// Must be absolute path
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("directory must be an absolute path")
	}

	// Check for path traversal (cleaned path should equal original)
	cleaned := filepath.Clean(dir)
	if cleaned != dir {
		return fmt.Errorf("invalid directory path")
	}

	return nil
}

// jsonError writes a JSON-formatted error response with the given status code.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("api: failed to encode error response: %v", err)
	}
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate all inputs
	if err := validateRouteName(req.Name); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateUpstream(req.Upstream); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateDir(req.Dir); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.registry.Register(req.Name, req.Upstream, req.Dir)
	if err != nil {
		if conflict, ok := errors.AsType[*ConflictError](err); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			if encErr := json.NewEncoder(w).Encode(map[string]string{
				"error":       "conflict",
				"existingDir": conflict.ExistingDir,
			}); encErr != nil {
				log.Printf("api: failed to encode conflict response: %v", encErr)
			}
			return
		}
		if limit, ok := errors.AsType[*LimitError](err); ok {
			jsonError(w, fmt.Sprintf("route limit reached (%d)", limit.Limit), http.StatusTooManyRequests)
			return
		}
		jsonError(w, "registration failed", http.StatusInternalServerError)
		return
	}

	if !req.Tailnet {
		w.WriteHeader(http.StatusOK)
		return
	}
	var resp RegisterResponse
	if s.tailnet == nil {
		resp.TailnetError = "tailnet serving is not available"
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), tailnetTimeout)
		defer cancel()
		url, err := s.tailnet.Expose(ctx, req.Name, req.Upstream)
		if err != nil {
			resp.TailnetError = err.Error()
		} else {
			resp.TailnetURL = url
			s.registry.SetTailnetURL(req.Name, url)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("api: failed to encode register response: %v", err)
	}
}

func (s *Server) handleDeregister(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := validateRouteName(name); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	route, _ := s.registry.Lookup(name)
	if s.registry.Deregister(name) {
		if route.TailnetURL != "" && s.tailnet != nil {
			ctx, cancel := context.WithTimeout(r.Context(), tailnetTimeout)
			defer cancel()
			s.tailnet.Unexpose(ctx, name)
		}
		w.WriteHeader(http.StatusOK)
	} else {
		jsonError(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := validateRouteName(name); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.registry.Heartbeat(name); err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	routes := s.registry.List()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(routes); err != nil {
		log.Printf("api: failed to encode route list response: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": Version,
		"build":   Build,
		"uptime":  uptime.String(),
	}); err != nil {
		log.Printf("api: failed to encode health response: %v", err)
	}
}
