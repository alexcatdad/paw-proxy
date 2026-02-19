# Web Dashboard for paw-proxy

**Date:** 2026-02-19
**Status:** Approved

## Summary

Add a live web dashboard served by the daemon at `https://_paw.test`. Shows active routes, per-route metrics, and a real-time request feed via SSE. Built with embedded HTML/CSS/JS — no external dependencies, no build step, single binary.

## Motivation

paw-proxy currently logs structured JSON to a file. `paw-proxy logs --tail` shows raw JSON, which is hard to scan. Developers need a quick way to see what's happening across all their local dev servers — which routes are active, what requests are flowing, and where errors are occurring. A browser-based dashboard is always one tab away and requires zero setup.

## Architecture

### Reserved Hostname: `_paw.test`

- The underscore prefix makes it visually distinct from app routes
- `_paw` is invalid as a DNS label (RFC 952), so it can never collide with a user route
- The daemon's route name regex `^[a-zA-Z][a-zA-Z0-9_-]{0,62}$` prevents `up` from registering `_paw`
- DNS resolves `_paw.test` to `127.0.0.1` (existing wildcard `*.test` behavior)
- TLS cert is auto-generated on first request (existing cert cache behavior)

### Request Flow

```
Browser → https://_paw.test → daemon HTTPS handler
  ├── host == "_paw.test" → dashboard handler (embedded HTML/SSE)
  └── else → normal proxy flow (existing behavior, unchanged)
```

### Data Layer: In-Memory Ring Buffer

- Fixed-size ring buffer (1000 entries) stores recent request metadata
- Each entry: timestamp, host, method, path, status, latency_ms, route, upstream
- No persistence — resets on daemon restart
- Per-route counters maintained alongside the buffer (request count, total latency, error count)

### Live Updates: Server-Sent Events (SSE)

- Endpoint: `/_paw/events`
- Pushes new request entries as JSON events
- Browser `EventSource` auto-reconnects on disconnect
- Fan-out to multiple browser tabs via subscriber map
- Cleanup on client disconnect via `r.Context().Done()`

## UI Layout

### Panel 1: Header Bar
- Left: paw-proxy name + version
- Right: daemon uptime, CA expiry, SSE connection status indicator

### Panel 2: Active Routes Table

| Route | Upstream | Dir | Uptime | Reqs | Avg Latency | Errors |
|-------|----------|-----|--------|------|-------------|--------|
| myapp.test | localhost:3000 | ~/myapp | 12m | 47 | 23ms | 0 |

- Route names are clickable links to the app
- Color-coded health: green (healthy), amber (slow), red (errors)
- Auto-updates when routes register/deregister

### Panel 3: Live Request Feed

Scrolling log of recent requests, newest on top:
- Color-coded by status: 2xx green, 3xx blue, 4xx amber, 5xx red
- Filterable by route (click route in table to filter)
- Pause/resume button
- Shows ~200 entries on screen, backed by 1000-entry ring buffer

### Styling
- Dark theme default, `prefers-color-scheme` support for light
- Monospace font for request feed
- CSS custom properties for theming
- No frameworks, no build step

## New Files

```
internal/
├── dashboard/
│   ├── dashboard.go       # HTTP handlers: HTML, SSE, JSON APIs
│   ├── dashboard_test.go  # Handler tests
│   ├── metrics.go         # Ring buffer + per-route counters + SSE fan-out
│   ├── metrics_test.go    # Buffer wrap, counter accuracy, subscriber lifecycle
│   └── static/
│       ├── index.html     # Single-page dashboard
│       ├── style.css      # Dark/light theme
│       └── app.js         # SSE client, DOM updates, filtering
```

## Changes to Existing Files

### `internal/daemon/daemon.go`

1. Add `dashboard` field to `Daemon` struct
2. In `New()`: create dashboard instance with metrics
3. In `handleRequest()`:
   - Before proxy: check if host is `_paw.test`, delegate to dashboard
   - After proxy: record request entry in metrics ring buffer
4. Pass route/status/latency to `metrics.Record()` after each proxied request

### No changes needed to:
- `internal/dns/` — wildcard `*.test` already resolves `_paw.test`
- `internal/ssl/` — cert cache already generates certs for any `.test` domain
- `internal/api/` — unix socket API is unchanged
- `cmd/up/` — no interaction with dashboard

## Dashboard API Endpoints

All served under `_paw.test`:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | HTML dashboard page |
| GET | `/events` | SSE stream of new requests |
| GET | `/api/routes` | Routes with metrics (JSON) |
| GET | `/api/stats` | Aggregate stats (JSON) |

## Key Data Structures

### RequestEntry
```go
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
```

### Metrics
```go
type Metrics struct {
    mu      sync.RWMutex
    entries []RequestEntry       // ring buffer
    pos     int
    count   int
    routes  map[string]*RouteMetrics  // per-route counters
    subs    map[chan RequestEntry]struct{}  // SSE subscribers
}

type RouteMetrics struct {
    Requests    int64
    TotalMs     int64
    Errors      int64   // 5xx count
    LastSeen    time.Time
}
```

## Testing Strategy

- **`metrics_test.go`**: ring buffer add/read, overflow wrap, per-route counter accuracy, SSE subscriber fan-out and cleanup
- **`dashboard_test.go`**: HTTP handler responses (HTML content type, SSE headers, JSON API format)
- **Integration**: add `curl -s https://_paw.test` to `integration-tests.sh`

## Constraints

- No external Go dependencies (stdlib + embed only)
- No JavaScript build step (vanilla JS)
- No persistent storage (in-memory only)
- Dashboard requests are not recorded in their own metrics (avoid feedback loop)
- Ring buffer size is hardcoded (1000) — not configurable for v1
