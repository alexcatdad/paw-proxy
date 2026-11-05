package main

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/alexcatdad/paw-proxy/internal/api"
)

// reportNotification tells the daemon about a route event so Paw Proxy.app
// can show it. Best effort: an older daemon without the endpoint, or a
// failure, never interrupts the dev server.
func reportNotification(client *http.Client, kind string, routes ...string) {
	if len(routes) > api.MaxNotificationRoutes {
		routes = routes[:api.MaxNotificationRoutes]
	}
	body, err := json.Marshal(api.NotificationRequest{Kind: kind, Routes: routes})
	if err != nil {
		return
	}
	resp, err := client.Post("http://unix/notifications", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
}
