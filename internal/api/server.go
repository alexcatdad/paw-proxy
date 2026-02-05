// internal/api/server.go
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"
)

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

	// Make socket accessible
	os.Chmod(s.socketPath, 0666)

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

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeregister(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
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
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	if err := s.registry.Heartbeat(name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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
