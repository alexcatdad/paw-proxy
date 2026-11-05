package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexcatdad/paw-proxy/internal/api"
)

func TestReportNotificationPostsEvent(t *testing.T) {
	got := make(chan api.NotificationRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/notifications" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req api.NotificationRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		got <- req
	}))
	defer server.Close()

	routes := make([]string, api.MaxNotificationRoutes+5)
	for i := range routes {
		routes[i] = "svc"
	}
	reportNotification(unixHostClient(t, server), api.NotificationRoutesLive, routes...)

	req := <-got
	if req.Kind != api.NotificationRoutesLive || len(req.Routes) != api.MaxNotificationRoutes {
		t.Fatalf("unexpected request: kind %q, %d routes", req.Kind, len(req.Routes))
	}
}

func TestReportNotificationIgnoresUnreachableDaemon(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close() // unreachable daemon must not panic or block
	reportNotification(unixHostClient(t, server), api.NotificationRoutesRemoved, "myapp")
}
