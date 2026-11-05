package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Notification kinds. `up` reports them; Paw Proxy.app polls and posts them
// from its own bundle, so nothing here ever shells out to show a notification.
const (
	NotificationRoutesLive    = "routes.live"
	NotificationRoutesRemoved = "routes.removed"
)

const (
	// notificationQueueSize bounds memory: the app only needs recent events.
	notificationQueueSize = 64
	// MaxNotificationRoutes caps one event (compose projects list many services).
	MaxNotificationRoutes = 32
)

// Notification is one event for the app to show. Text is composed by the app
// from Kind and Routes, so clients never inject display strings.
type Notification struct {
	ID     uint64    `json:"id"`
	Kind   string    `json:"kind"`
	Routes []string  `json:"routes"`
	Time   time.Time `json:"time"`
}

// NotificationQueue is a bounded, in-memory FIFO with monotonically increasing
// IDs. Readers pass the last ID they saw and get everything newer.
type NotificationQueue struct {
	mu     sync.Mutex
	items  []Notification
	nextID uint64
	limit  int
}

func NewNotificationQueue(limit int) *NotificationQueue {
	return &NotificationQueue{limit: limit, nextID: 1}
}

// Add appends an event, dropping the oldest when the queue is full.
func (q *NotificationQueue) Add(kind string, routes []string) Notification {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := Notification{ID: q.nextID, Kind: kind, Routes: append([]string(nil), routes...), Time: time.Now()}
	q.nextID++
	q.items = append(q.items, n)
	if len(q.items) > q.limit {
		q.items = append([]Notification(nil), q.items[len(q.items)-q.limit:]...)
	}
	return n
}

// Since returns the events with ID greater than after, and the cursor to pass
// next time (the newest ID issued so far).
func (q *NotificationQueue) Since(after uint64) ([]Notification, uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := []Notification{}
	for _, n := range q.items {
		if n.ID > after {
			out = append(out, n)
		}
	}
	return out, q.nextID - 1
}

type NotificationRequest struct {
	Kind   string   `json:"kind"`
	Routes []string `json:"routes"`
}

func validateNotification(req NotificationRequest) error {
	if req.Kind != NotificationRoutesLive && req.Kind != NotificationRoutesRemoved {
		return fmt.Errorf("invalid kind: must be %q or %q", NotificationRoutesLive, NotificationRoutesRemoved)
	}
	if len(req.Routes) == 0 || len(req.Routes) > MaxNotificationRoutes {
		return fmt.Errorf("routes: must list 1-%d route names", MaxNotificationRoutes)
	}
	for _, name := range req.Routes {
		if err := validateRouteName(name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req NotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateNotification(req); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.notifications.Add(req.Kind, req.Routes)
	w.WriteHeader(http.StatusOK)
}

type notificationsResponse struct {
	Cursor        uint64         `json:"cursor"`
	Notifications []Notification `json:"notifications"`
}

// handleNotifications serves GET /notifications?after=<cursor>.
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	var after uint64
	if v := r.URL.Query().Get("after"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			jsonError(w, "invalid after: must be a non-negative integer", http.StatusBadRequest)
			return
		}
		after = parsed
	}
	items, cursor := s.notifications.Since(after)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(notificationsResponse{Cursor: cursor, Notifications: items}); err != nil {
		log.Printf("api: failed to encode notifications response: %v", err)
	}
}
