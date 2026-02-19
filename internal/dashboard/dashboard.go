package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/api"
)

//go:embed static
var staticFS embed.FS

// RouteProvider abstracts access to the route registry.
type RouteProvider interface {
	List() []api.Route
}

// Dashboard serves the web dashboard UI and its API endpoints.
type Dashboard struct {
	metrics   *Metrics
	routes    RouteProvider
	version   string
	startTime time.Time
	mux       *http.ServeMux
}

// New creates a Dashboard instance.
func New(metrics *Metrics, routes RouteProvider, version string, startTime time.Time) (*Dashboard, error) {
	d := &Dashboard{
		metrics:   metrics,
		routes:    routes,
		version:   version,
		startTime: startTime,
	}

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("dashboard: create sub filesystem: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /events", d.handleEvents)
	mux.HandleFunc("GET /api/routes", d.handleAPIRoutes)
	mux.HandleFunc("GET /api/stats", d.handleAPIStats)
	mux.Handle("GET /", http.FileServerFS(staticSub))

	d.mux = mux
	return d, nil
}

func (d *Dashboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mux.ServeHTTP(w, r)
}

type routeWithMetrics struct {
	Name       string    `json:"name"`
	Upstream   string    `json:"upstream"`
	Dir        string    `json:"dir"`
	Registered time.Time `json:"registered"`
	Requests   int64     `json:"requests"`
	AvgMs      int64     `json:"avgMs"`
	Errors     int64     `json:"errors"`
}

func (d *Dashboard) handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	routes := d.routes.List()
	stats := d.metrics.RouteStats()

	result := make([]routeWithMetrics, 0, len(routes))
	for _, route := range routes {
		rm := routeWithMetrics{
			Name:       route.Name,
			Upstream:   route.Upstream,
			Dir:        route.Dir,
			Registered: route.Registered,
		}
		if s, ok := stats[route.Name]; ok {
			rm.Requests = s.Requests
			rm.Errors = s.Errors
			if s.Requests > 0 {
				rm.AvgMs = s.TotalMs / s.Requests
			}
		}
		result = append(result, rm)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("dashboard: failed to encode routes: %v", err)
	}
}

func (d *Dashboard) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(d.startTime)
	uptimeStr := formatDuration(uptime)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"version": d.version,
		"uptime":  uptimeStr,
	}); err != nil {
		log.Printf("dashboard: failed to encode stats: %v", err)
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func (d *Dashboard) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := d.metrics.Subscribe()
	defer d.metrics.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
