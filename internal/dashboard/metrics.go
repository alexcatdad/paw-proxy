package dashboard

import (
	"sync"
	"time"
)

type RequestEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Host       string    `json:"host"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"statusCode"`
	LatencyMs  int64     `json:"latencyMs"`
	Route      string    `json:"route"`
	Upstream   string    `json:"upstream"`
}

type RouteMetrics struct {
	Requests int64     `json:"requests"`
	TotalMs  int64     `json:"totalMs"`
	Errors   int64     `json:"errors"`
	LastSeen time.Time `json:"lastSeen"`
}

type Metrics struct {
	mu      sync.RWMutex
	entries []RequestEntry
	pos     int
	count   int
	routes  map[string]*RouteMetrics
	subsMu  sync.Mutex
	subs    map[chan RequestEntry]struct{}
}

func NewMetrics(bufferSize int) *Metrics {
	return &Metrics{
		entries: make([]RequestEntry, bufferSize),
		routes:  make(map[string]*RouteMetrics),
		subs:    make(map[chan RequestEntry]struct{}),
	}
}

func (m *Metrics) Record(entry RequestEntry) {
	m.mu.Lock()
	m.entries[m.pos] = entry
	m.pos = (m.pos + 1) % len(m.entries)
	if m.count < len(m.entries) {
		m.count++
	}
	if entry.Route != "" {
		rm, ok := m.routes[entry.Route]
		if !ok {
			rm = &RouteMetrics{}
			m.routes[entry.Route] = rm
		}
		rm.Requests++
		rm.TotalMs += entry.LatencyMs
		if entry.StatusCode >= 500 {
			rm.Errors++
		}
		rm.LastSeen = entry.Timestamp
	}
	m.mu.Unlock()

	m.subsMu.Lock()
	for ch := range m.subs {
		select {
		case ch <- entry:
		default:
		}
	}
	m.subsMu.Unlock()
}

func (m *Metrics) Recent(n int) []RequestEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if n > m.count {
		n = m.count
	}
	result := make([]RequestEntry, n)
	for i := 0; i < n; i++ {
		idx := (m.pos - 1 - i + len(m.entries)) % len(m.entries)
		result[i] = m.entries[idx]
	}
	return result
}

func (m *Metrics) RouteStats() map[string]RouteMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]RouteMetrics, len(m.routes))
	for name, rm := range m.routes {
		result[name] = *rm
	}
	return result
}

func (m *Metrics) Subscribe() chan RequestEntry {
	ch := make(chan RequestEntry, 64)
	m.subsMu.Lock()
	m.subs[ch] = struct{}{}
	m.subsMu.Unlock()
	return ch
}

func (m *Metrics) Unsubscribe(ch chan RequestEntry) {
	m.subsMu.Lock()
	delete(m.subs, ch)
	m.subsMu.Unlock()
}
