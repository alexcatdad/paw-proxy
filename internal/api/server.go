// internal/api/server.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// Max request body size (1MB)
const maxRequestBodySize = 1024 * 1024

// Route name validation pattern: alphanumeric, dash, underscore only
var routeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

type Server struct {
	socketPath string
	registry   *RouteRegistry
	server     *http.Server
	listener   net.Listener
	startTime  time.Time
}

func NewServer(socketPath string, registry *RouteRegistry) *Server {
	s := &Server{
		socketPath: socketPath,
		registry:   registry,
		startTime:  time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /routes", s.handleRegister)
	mux.HandleFunc("DELETE /routes/{name}", s.handleDeregister)
	mux.HandleFunc("POST /routes/{name}/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /routes", s.handleList)
	mux.HandleFunc("GET /health", s.handleHealth)

	s.server = &http.Server{Handler: mux}

	return s
}

func (s *Server) Start() error {
	// Remove existing socket
	os.Remove(s.socketPath)

	var err error
	s.listener, err = net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}

	// SECURITY: Owner-only access to prevent privilege escalation
	os.Chmod(s.socketPath, 0600)

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
}

// validateRouteName ensures route names are safe for DNS, filesystem, and shell use
func validateRouteName(name string) error {
	if !routeNamePattern.MatchString(name) {
		return fmt.Errorf("invalid route name: must be 1-63 alphanumeric characters, dashes, or underscores")
	}
	return nil
}

// validateUpstream ensures upstream targets are localhost only (prevent SSRF)
func validateUpstream(upstream string) error {
	host, portStr, err := net.SplitHostPort(upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream format: expected host:port")
	}

	// SECURITY: Only allow localhost/loopback to prevent SSRF
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("upstream must be localhost, 127.0.0.1, or ::1")
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

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate all inputs
	if err := validateRouteName(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateUpstream(req.Upstream); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateDir(req.Dir); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.registry.Register(req.Name, req.Upstream, req.Dir)
	if err != nil {
		if conflict, ok := err.(*ConflictError); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error":       "conflict",
				"existingDir": conflict.ExistingDir,
			})
			return
		}
		http.Error(w, "registration failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeregister(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := validateRouteName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.registry.Deregister(name) {
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := validateRouteName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.registry.Heartbeat(name); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	routes := s.registry.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "1.0.0",
		"uptime":  uptime.String(),
	})
}
