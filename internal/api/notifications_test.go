package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestNotificationQueue_SinceAndCursor(t *testing.T) {
	q := NewNotificationQueue(3)
	if items, cursor := q.Since(0); len(items) != 0 || cursor != 0 {
		t.Fatalf("empty queue: got %d items, cursor %d", len(items), cursor)
	}
	q.Add(NotificationRoutesLive, []string{"a"})
	q.Add(NotificationRoutesLive, []string{"b"})
	items, cursor := q.Since(0)
	if len(items) != 2 || cursor != 2 || items[0].ID != 1 || items[1].Routes[0] != "b" {
		t.Fatalf("unexpected: %+v cursor %d", items, cursor)
	}
	if items, _ := q.Since(cursor); len(items) != 0 {
		t.Fatalf("expected nothing after cursor, got %+v", items)
	}
}

func TestNotificationQueue_DropsOldest(t *testing.T) {
	q := NewNotificationQueue(2)
	for _, n := range []string{"a", "b", "c"} {
		q.Add(NotificationRoutesRemoved, []string{n})
	}
	items, cursor := q.Since(0)
	if len(items) != 2 || items[0].ID != 2 || cursor != 3 {
		t.Fatalf("unexpected: %+v cursor %d", items, cursor)
	}
}

func TestNotificationQueue_CopiesRoutes(t *testing.T) {
	q := NewNotificationQueue(2)
	routes := []string{"a"}
	q.Add(NotificationRoutesLive, routes)
	routes[0] = "mutated"
	items, _ := q.Since(0)
	if items[0].Routes[0] != "a" {
		t.Fatalf("queue shares caller's slice: %v", items[0].Routes)
	}
}

func TestValidateNotification(t *testing.T) {
	many := make([]string, MaxNotificationRoutes+1)
	for i := range many {
		many[i] = "svc"
	}
	tests := []struct {
		name string
		req  NotificationRequest
		ok   bool
	}{
		{"live", NotificationRequest{Kind: NotificationRoutesLive, Routes: []string{"myapp"}}, true},
		{"removed", NotificationRequest{Kind: NotificationRoutesRemoved, Routes: []string{"a", "b"}}, true},
		{"unknown kind", NotificationRequest{Kind: "shell", Routes: []string{"myapp"}}, false},
		{"no routes", NotificationRequest{Kind: NotificationRoutesLive}, false},
		{"too many routes", NotificationRequest{Kind: NotificationRoutesLive, Routes: many}, false},
		{"bad route name", NotificationRequest{Kind: NotificationRoutesLive, Routes: []string{`"; do shell script "x`}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNotification(tt.req); (err == nil) != tt.ok {
				t.Fatalf("validateNotification(%+v) = %v, want ok=%v", tt.req, err, tt.ok)
			}
		})
	}
}

func TestAPIServer_Notifications(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	srv := NewServer(socketPath, NewRouteRegistry(30*time.Second))
	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Stop() }()
	time.Sleep(50 * time.Millisecond)
	client := testSocketClient(socketPath)

	post := func(req NotificationRequest) int {
		body, _ := json.Marshal(req)
		resp, err := client.Post("http://unix/notifications", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST /notifications: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	get := func(after string) (int, notificationsResponse) {
		resp, err := client.Get("http://unix/notifications" + after)
		if err != nil {
			t.Fatalf("GET /notifications: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out notificationsResponse
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return resp.StatusCode, out
	}

	if code, out := get(""); code != http.StatusOK || out.Cursor != 0 || out.Notifications == nil || len(out.Notifications) != 0 {
		t.Fatalf("empty GET: code %d, %+v", code, out)
	}
	if code := post(NotificationRequest{Kind: NotificationRoutesLive, Routes: []string{"myapp"}}); code != http.StatusOK {
		t.Fatalf("valid POST: got %d", code)
	}
	if code := post(NotificationRequest{Kind: "bogus", Routes: []string{"myapp"}}); code != http.StatusBadRequest {
		t.Fatalf("invalid POST: got %d", code)
	}
	code, out := get("?after=0")
	if code != http.StatusOK || out.Cursor != 1 || len(out.Notifications) != 1 || out.Notifications[0].Kind != NotificationRoutesLive {
		t.Fatalf("GET after POST: code %d, %+v", code, out)
	}
	if code, out := get("?after=1"); code != http.StatusOK || len(out.Notifications) != 0 {
		t.Fatalf("GET after cursor: code %d, %+v", code, out)
	}
	if code, _ := get("?after=-1"); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: got %d", code)
	}
}

func testSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}
