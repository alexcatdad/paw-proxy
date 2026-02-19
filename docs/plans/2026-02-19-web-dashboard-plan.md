# Web Dashboard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a live web dashboard at `https://_paw.test` showing active routes, per-route metrics, and a real-time request feed via SSE.

**Architecture:** The daemon intercepts requests for `_paw.test` and serves an embedded HTML/CSS/JS dashboard. A new `internal/dashboard` package provides an in-memory ring buffer for request metrics and SSE fan-out for live updates. A `statusCapture` ResponseWriter wrapper in the daemon captures upstream status codes for accurate metrics.

**Tech Stack:** Go stdlib (`embed`, `net/http`, `sync`, `encoding/json`), vanilla HTML/CSS/JS, SSE (`text/event-stream`).

**Design doc:** `docs/plans/2026-02-19-web-dashboard-design.md`

---

### Task 1: Metrics Ring Buffer + SSE Subscribers

**Files:**
- Create: `internal/dashboard/metrics.go`
- Create: `internal/dashboard/metrics_test.go`

**Step 1: Write the failing tests**

```go
// internal/dashboard/metrics_test.go
package dashboard

import (
	"sync"
	"testing"
	"time"
)

func makeEntry(route string, status int, latencyMs int64) RequestEntry {
	return RequestEntry{
		Timestamp:  time.Now(),
		Host:       route + ".test",
		Method:     "GET",
		Path:       "/",
		StatusCode: status,
		LatencyMs:  latencyMs,
		Route:      route,
		Upstream:   "localhost:3000",
	}
}

func TestMetrics_RecordAndRecent(t *testing.T) {
	m := NewMetrics(10)

	m.Record(makeEntry("app", 200, 10))
	m.Record(makeEntry("app", 200, 20))

	entries := m.Recent(10)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Recent returns newest first
	if entries[0].LatencyMs != 20 {
		t.Errorf("expected newest first (latency 20), got %d", entries[0].LatencyMs)
	}
	if entries[1].LatencyMs != 10 {
		t.Errorf("expected oldest second (latency 10), got %d", entries[1].LatencyMs)
	}
}

func TestMetrics_RecentClampsToCount(t *testing.T) {
	m := NewMetrics(10)

	m.Record(makeEntry("app", 200, 10))

	entries := m.Recent(100) // ask for more than exist
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMetrics_RingBufferWraparound(t *testing.T) {
	m := NewMetrics(3) // small buffer

	for i := 0; i < 5; i++ {
		m.Record(makeEntry("app", 200, int64(i)))
	}

	entries := m.Recent(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (buffer size), got %d", len(entries))
	}
	// Should have entries 4, 3, 2 (newest first)
	if entries[0].LatencyMs != 4 {
		t.Errorf("expected newest latency 4, got %d", entries[0].LatencyMs)
	}
	if entries[2].LatencyMs != 2 {
		t.Errorf("expected oldest latency 2, got %d", entries[2].LatencyMs)
	}
}

func TestMetrics_RouteStats(t *testing.T) {
	m := NewMetrics(100)

	m.Record(makeEntry("app", 200, 10))
	m.Record(makeEntry("app", 200, 30))
	m.Record(makeEntry("api", 200, 50))
	m.Record(makeEntry("app", 500, 100))

	stats := m.RouteStats()
	appStats, ok := stats["app"]
	if !ok {
		t.Fatal("expected stats for 'app'")
	}
	if appStats.Requests != 3 {
		t.Errorf("expected 3 requests for app, got %d", appStats.Requests)
	}
	if appStats.TotalMs != 140 {
		t.Errorf("expected 140ms total for app, got %d", appStats.TotalMs)
	}
	if appStats.Errors != 1 {
		t.Errorf("expected 1 error for app, got %d", appStats.Errors)
	}

	apiStats, ok := stats["api"]
	if !ok {
		t.Fatal("expected stats for 'api'")
	}
	if apiStats.Requests != 1 {
		t.Errorf("expected 1 request for api, got %d", apiStats.Requests)
	}
	if apiStats.Errors != 0 {
		t.Errorf("expected 0 errors for api, got %d", apiStats.Errors)
	}
}

func TestMetrics_SubscribeReceivesNewEntries(t *testing.T) {
	m := NewMetrics(10)
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	entry := makeEntry("app", 200, 10)
	m.Record(entry)

	select {
	case got := <-ch:
		if got.Route != "app" {
			t.Errorf("expected route 'app', got %q", got.Route)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber to receive entry")
	}
}

func TestMetrics_UnsubscribeStopsDelivery(t *testing.T) {
	m := NewMetrics(10)
	ch := m.Subscribe()
	m.Unsubscribe(ch)

	m.Record(makeEntry("app", 200, 10))

	select {
	case <-ch:
		t.Fatal("should not receive after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected: no delivery
	}
}

func TestMetrics_SlowSubscriberDoesNotBlock(t *testing.T) {
	m := NewMetrics(10)
	_ = m.Subscribe() // subscribe but never read

	// Record should not block even with a slow subscriber
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			m.Record(makeEntry("app", 200, int64(i)))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked due to slow subscriber")
	}
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	m := NewMetrics(100)
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					m.Record(makeEntry("app", 200, int64(id)))
				}
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					m.Recent(10)
					m.RouteStats()
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v -race ./internal/dashboard/`
Expected: FAIL — package does not exist yet.

**Step 3: Write minimal implementation**

```go
// internal/dashboard/metrics.go
package dashboard

import (
	"sync"
	"time"
)

// RequestEntry represents a single proxied request for metrics tracking.
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

// RouteMetrics holds aggregate counters for a single route.
type RouteMetrics struct {
	Requests int64     `json:"requests"`
	TotalMs  int64     `json:"totalMs"`
	Errors   int64     `json:"errors"`
	LastSeen time.Time `json:"lastSeen"`
}

// Metrics provides an in-memory ring buffer of recent requests with
// per-route aggregate counters and SSE subscriber fan-out.
type Metrics struct {
	mu      sync.RWMutex
	entries []RequestEntry
	pos     int
	count   int

	routes map[string]*RouteMetrics

	subsMu sync.Mutex
	subs   map[chan RequestEntry]struct{}
}

// NewMetrics creates a Metrics instance with the given ring buffer capacity.
func NewMetrics(bufferSize int) *Metrics {
	return &Metrics{
		entries: make([]RequestEntry, bufferSize),
		routes:  make(map[string]*RouteMetrics),
		subs:    make(map[chan RequestEntry]struct{}),
	}
}

// Record adds an entry to the ring buffer, updates per-route stats,
// and fans out to all SSE subscribers.
func (m *Metrics) Record(entry RequestEntry) {
	m.mu.Lock()
	m.entries[m.pos] = entry
	m.pos = (m.pos + 1) % len(m.entries)
	if m.count < len(m.entries) {
		m.count++
	}

	// Update per-route counters
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

	// Fan-out to SSE subscribers (non-blocking send)
	m.subsMu.Lock()
	for ch := range m.subs {
		select {
		case ch <- entry:
		default:
			// Drop if subscriber is slow — prevents blocking
		}
	}
	m.subsMu.Unlock()
}

// Recent returns the last n entries, newest first.
func (m *Metrics) Recent(n int) []RequestEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if n > m.count {
		n = m.count
	}
	result := make([]RequestEntry, n)
	for i := 0; i < n; i++ {
		// Walk backwards from most recent
		idx := (m.pos - 1 - i + len(m.entries)) % len(m.entries)
		result[i] = m.entries[idx]
	}
	return result
}

// RouteStats returns a copy of per-route metrics.
func (m *Metrics) RouteStats() map[string]RouteMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]RouteMetrics, len(m.routes))
	for name, rm := range m.routes {
		result[name] = *rm
	}
	return result
}

// Subscribe returns a buffered channel that receives new RequestEntry values
// as they are recorded. Call Unsubscribe to stop delivery and clean up.
func (m *Metrics) Subscribe() chan RequestEntry {
	ch := make(chan RequestEntry, 64)
	m.subsMu.Lock()
	m.subs[ch] = struct{}{}
	m.subsMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (m *Metrics) Unsubscribe(ch chan RequestEntry) {
	m.subsMu.Lock()
	delete(m.subs, ch)
	close(ch)
	m.subsMu.Unlock()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -race ./internal/dashboard/`
Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/dashboard/metrics.go internal/dashboard/metrics_test.go
git commit -m "feat(dashboard): add metrics ring buffer with SSE subscriber support"
```

---

### Task 2: Static Files (HTML/CSS/JS)

**Files:**
- Create: `internal/dashboard/static/index.html`
- Create: `internal/dashboard/static/style.css`
- Create: `internal/dashboard/static/app.js`

These are not TDD-driven — they're the frontend UI consumed by the embedded Go file server.

**Step 1: Create `index.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>paw-proxy</title>
<link rel="stylesheet" href="/style.css">
</head>
<body>
<header>
  <div class="header-left">
    <h1>paw-proxy</h1>
    <span id="version" class="badge"></span>
  </div>
  <div class="header-right">
    <span id="uptime"></span>
    <span id="sse-dot" class="dot dot-off" title="SSE disconnected"></span>
  </div>
</header>

<section id="routes-section">
  <h2>Active Routes</h2>
  <table id="routes-table">
    <thead>
      <tr>
        <th>Route</th>
        <th>Upstream</th>
        <th>Dir</th>
        <th>Uptime</th>
        <th>Reqs</th>
        <th>Avg</th>
        <th>Errors</th>
      </tr>
    </thead>
    <tbody id="routes-body"></tbody>
  </table>
  <p id="no-routes" class="muted">No active routes. Start a dev server with <code>up &lt;command&gt;</code></p>
</section>

<section id="feed-section">
  <div class="feed-header">
    <h2>Request Feed</h2>
    <div class="feed-controls">
      <span id="filter-label" hidden>
        Filtering: <strong id="filter-name"></strong>
        <button id="clear-filter" class="btn-small">&times;</button>
      </span>
      <button id="pause-btn" class="btn-small">Pause</button>
    </div>
  </div>
  <div id="feed-list"></div>
</section>

<script src="/app.js"></script>
</body>
</html>
```

**Step 2: Create `style.css`**

```css
:root {
  --bg: #1a1a2e;
  --bg-surface: #16213e;
  --bg-hover: #1f2b47;
  --text: #e0e0e0;
  --text-muted: #8892a4;
  --border: #2a2a4a;
  --accent: #4fc3f7;
  --green: #66bb6a;
  --blue: #42a5f5;
  --amber: #ffa726;
  --red: #ef5350;
  --mono: "SF Mono", "Cascadia Code", "Fira Code", Consolas, monospace;
}

@media (prefers-color-scheme: light) {
  :root {
    --bg: #f5f5f5;
    --bg-surface: #ffffff;
    --bg-hover: #eef2f7;
    --text: #1a1a2e;
    --text-muted: #6b7280;
    --border: #d1d5db;
    --accent: #0288d1;
    --green: #2e7d32;
    --blue: #1565c0;
    --amber: #e65100;
    --red: #c62828;
  }
}

* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: -apple-system, system-ui, "Segoe UI", sans-serif;
  background: var(--bg);
  color: var(--text);
  max-width: 1200px;
  margin: 0 auto;
  padding: 16px 24px;
}

header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid var(--border);
  margin-bottom: 24px;
}

.header-left { display: flex; align-items: center; gap: 12px; }
.header-right { display: flex; align-items: center; gap: 16px; color: var(--text-muted); font-size: 14px; }

h1 { font-size: 20px; font-weight: 600; }
h2 { font-size: 16px; font-weight: 600; margin-bottom: 12px; }

.badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--bg-hover);
  color: var(--text-muted);
}

.dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  display: inline-block;
}
.dot-on { background: var(--green); }
.dot-off { background: var(--red); }

section { margin-bottom: 32px; }

table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

th {
  text-align: left;
  padding: 8px 12px;
  border-bottom: 2px solid var(--border);
  color: var(--text-muted);
  font-weight: 500;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
}

tr:hover td { background: var(--bg-hover); }

tr.clickable { cursor: pointer; }

td a {
  color: var(--accent);
  text-decoration: none;
}
td a:hover { text-decoration: underline; }

.muted { color: var(--text-muted); font-size: 14px; }

code {
  font-family: var(--mono);
  background: var(--bg-hover);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}

.feed-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.feed-controls { display: flex; align-items: center; gap: 12px; font-size: 13px; }

.btn-small {
  font-size: 12px;
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--bg-surface);
  color: var(--text);
  cursor: pointer;
}
.btn-small:hover { background: var(--bg-hover); }

#feed-list {
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.6;
  max-height: 500px;
  overflow-y: auto;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 8px;
}

.feed-entry {
  display: flex;
  gap: 12px;
  padding: 2px 4px;
  border-radius: 3px;
}
.feed-entry:hover { background: var(--bg-hover); }

.feed-time { color: var(--text-muted); min-width: 70px; }
.feed-method { min-width: 50px; font-weight: 600; }
.feed-host { color: var(--accent); min-width: 140px; }
.feed-path { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.feed-status { min-width: 36px; text-align: right; font-weight: 600; }
.feed-latency { min-width: 50px; text-align: right; color: var(--text-muted); }

.status-2xx { color: var(--green); }
.status-3xx { color: var(--blue); }
.status-4xx { color: var(--amber); }
.status-5xx { color: var(--red); }

.errors-nonzero { color: var(--red); font-weight: 600; }
```

**Step 3: Create `app.js`**

SECURITY NOTE: All dynamic content is HTML-escaped via the `esc()` function
before DOM insertion. The `esc()` function uses `textContent` assignment
followed by `innerHTML` read — a standard browser-safe escaping pattern.
Numeric values (status codes, latency, request counts) are inserted directly
since they cannot contain HTML.

```javascript
(function() {
  "use strict";

  var MAX_FEED = 200;
  var paused = false;
  var filterRoute = null;
  var pendingWhilePaused = [];

  var feedList = document.getElementById("feed-list");
  var pauseBtn = document.getElementById("pause-btn");
  var filterLabel = document.getElementById("filter-label");
  var filterNameEl = document.getElementById("filter-name");
  var clearFilter = document.getElementById("clear-filter");
  var sseDot = document.getElementById("sse-dot");
  var versionEl = document.getElementById("version");
  var uptimeEl = document.getElementById("uptime");
  var routesBody = document.getElementById("routes-body");
  var noRoutes = document.getElementById("no-routes");

  // SECURITY: HTML-escape dynamic strings before DOM insertion.
  // Uses textContent assignment (safe) then reads innerHTML (escaped).
  function esc(str) {
    var div = document.createElement("div");
    div.textContent = str;
    return div.innerHTML;
  }

  function fetchStats() {
    fetch("/api/stats")
      .then(function(r) { return r.json(); })
      .then(function(data) {
        versionEl.textContent = "v" + data.version;
        uptimeEl.textContent = "up " + data.uptime;
      })
      .catch(function() {});
  }

  function fetchRoutes() {
    fetch("/api/routes")
      .then(function(r) { return r.json(); })
      .then(function(routes) {
        routesBody.textContent = "";
        if (!routes || routes.length === 0) {
          noRoutes.hidden = false;
          return;
        }
        noRoutes.hidden = true;
        routes.forEach(function(route) {
          var tr = document.createElement("tr");
          tr.className = "clickable";
          tr.addEventListener("click", function() { setFilter(route.name); });

          var avgMs = route.requests > 0 ? Math.round(route.avgMs) : 0;

          // Build cells with safe DOM methods
          var cells = [
            createLinkCell(route.name + ".test", "https://" + route.name + ".test"),
            createTextCell(route.upstream),
            createTextCell(shortenDir(route.dir)),
            createTextCell(formatUptime(route.registered)),
            createTextCell(String(route.requests)),
            createTextCell(avgMs + "ms"),
            createErrorCell(route.errors)
          ];
          cells.forEach(function(td) { tr.appendChild(td); });
          routesBody.appendChild(tr);
        });
      })
      .catch(function() {});
  }

  function createTextCell(text) {
    var td = document.createElement("td");
    td.textContent = text;
    return td;
  }

  function createLinkCell(text, href) {
    var td = document.createElement("td");
    var a = document.createElement("a");
    a.textContent = text;
    a.href = href;
    a.target = "_blank";
    td.appendChild(a);
    return td;
  }

  function createErrorCell(errors) {
    var td = document.createElement("td");
    td.textContent = String(errors);
    if (errors > 0) td.className = "errors-nonzero";
    return td;
  }

  function shortenDir(dir) {
    var home = "/Users/";
    var idx = dir.indexOf(home);
    if (idx === 0) {
      var rest = dir.substring(home.length);
      var slashIdx = rest.indexOf("/");
      if (slashIdx !== -1) {
        return "~" + rest.substring(slashIdx);
      }
    }
    return dir;
  }

  function formatUptime(registered) {
    var ms = Date.now() - new Date(registered).getTime();
    var s = Math.floor(ms / 1000);
    if (s < 60) return s + "s";
    var m = Math.floor(s / 60);
    if (m < 60) return m + "m";
    var h = Math.floor(m / 60);
    return h + "h " + (m % 60) + "m";
  }

  function statusClass(code) {
    if (code >= 500) return "status-5xx";
    if (code >= 400) return "status-4xx";
    if (code >= 300) return "status-3xx";
    return "status-2xx";
  }

  function formatTime(ts) {
    var d = new Date(ts);
    return d.toLocaleTimeString("en-US", { hour12: false });
  }

  function addFeedEntry(entry) {
    if (filterRoute && entry.route !== filterRoute) return;

    var div = document.createElement("div");
    div.className = "feed-entry";

    var parts = [
      { cls: "feed-time", text: formatTime(entry.timestamp) },
      { cls: "feed-method", text: entry.method },
      { cls: "feed-host", text: entry.host },
      { cls: "feed-path", text: entry.path },
      { cls: "feed-status " + statusClass(entry.statusCode), text: String(entry.statusCode) },
      { cls: "feed-latency", text: entry.latencyMs + "ms" }
    ];

    parts.forEach(function(p) {
      var span = document.createElement("span");
      span.className = p.cls;
      span.textContent = p.text;
      div.appendChild(span);
    });

    feedList.insertBefore(div, feedList.firstChild);

    while (feedList.children.length > MAX_FEED) {
      feedList.removeChild(feedList.lastChild);
    }
  }

  // SSE connection
  function connectSSE() {
    var es = new EventSource("/events");

    es.onopen = function() {
      sseDot.className = "dot dot-on";
      sseDot.title = "SSE connected";
    };

    es.onmessage = function(event) {
      var entry = JSON.parse(event.data);
      if (paused) {
        pendingWhilePaused.push(entry);
        if (pendingWhilePaused.length > MAX_FEED) pendingWhilePaused.shift();
        return;
      }
      addFeedEntry(entry);
    };

    es.onerror = function() {
      sseDot.className = "dot dot-off";
      sseDot.title = "SSE disconnected — reconnecting...";
    };
  }

  // Pause/resume
  pauseBtn.addEventListener("click", function() {
    paused = !paused;
    pauseBtn.textContent = paused ? "Resume" : "Pause";
    if (!paused) {
      pendingWhilePaused.forEach(addFeedEntry);
      pendingWhilePaused = [];
    }
  });

  // Filtering
  function setFilter(route) {
    filterRoute = route;
    filterNameEl.textContent = route + ".test";
    filterLabel.hidden = false;
    feedList.textContent = "";
  }

  clearFilter.addEventListener("click", function() {
    filterRoute = null;
    filterLabel.hidden = true;
    feedList.textContent = "";
  });

  // Init
  fetchStats();
  fetchRoutes();
  connectSSE();
  setInterval(fetchRoutes, 5000);
  setInterval(fetchStats, 10000);
})();
```

**Step 4: Verify files exist**

Run: `ls internal/dashboard/static/`
Expected: `app.js  index.html  style.css`

**Step 5: Commit**

```bash
git add internal/dashboard/static/
git commit -m "feat(dashboard): add static HTML/CSS/JS for dashboard UI"
```

---

### Task 3: Dashboard HTTP Handlers

**Files:**
- Create: `internal/dashboard/dashboard.go`
- Create: `internal/dashboard/dashboard_test.go`

**Step 1: Write the failing tests**

```go
// internal/dashboard/dashboard_test.go
package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/api"
)

// mockRouteProvider implements the RouteProvider interface for testing.
type mockRouteProvider struct {
	routes []api.Route
}

func (m *mockRouteProvider) List() []api.Route {
	return m.routes
}

func TestDashboard_ServesHTML(t *testing.T) {
	d := New(NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content type, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "paw-proxy") {
		t.Error("expected 'paw-proxy' in HTML body")
	}
}

func TestDashboard_ServesCSS(t *testing.T) {
	d := New(NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/style.css", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "css") {
		t.Errorf("expected CSS content type, got %s", ct)
	}
}

func TestDashboard_ServesJS(t *testing.T) {
	d := New(NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/app.js", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "EventSource") {
		t.Error("expected JavaScript SSE code in body")
	}
}

func TestDashboard_APIRoutes(t *testing.T) {
	now := time.Now()
	routes := &mockRouteProvider{
		routes: []api.Route{
			{Name: "myapp", Upstream: "localhost:3000", Dir: "/home/user/myapp", Registered: now},
		},
	}
	m := NewMetrics(10)
	m.Record(RequestEntry{Timestamp: now, Route: "myapp", StatusCode: 200, LatencyMs: 25})
	m.Record(RequestEntry{Timestamp: now, Route: "myapp", StatusCode: 500, LatencyMs: 100})

	d := New(m, routes, "1.0.0", now)

	req := httptest.NewRequest("GET", "https://_paw.test/api/routes", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 route, got %d", len(result))
	}
	if result[0]["name"] != "myapp" {
		t.Errorf("expected name 'myapp', got %v", result[0]["name"])
	}
	if result[0]["requests"].(float64) != 2 {
		t.Errorf("expected 2 requests, got %v", result[0]["requests"])
	}
	if result[0]["errors"].(float64) != 1 {
		t.Errorf("expected 1 error, got %v", result[0]["errors"])
	}
}

func TestDashboard_APIStats(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Minute)
	d := New(NewMetrics(10), &mockRouteProvider{}, "1.2.3", startTime)

	req := httptest.NewRequest("GET", "https://_paw.test/api/stats", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if result["version"] != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %v", result["version"])
	}
	if _, ok := result["uptime"]; !ok {
		t.Error("expected 'uptime' field in stats")
	}
}

func TestDashboard_SSEHeaders(t *testing.T) {
	d := New(NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/events", nil)
	w := httptest.NewRecorder()

	// Run handler in goroutine since it blocks on context
	done := make(chan struct{})
	go func() {
		d.ServeHTTP(w, req)
		close(done)
	}()

	// Give it a moment to set headers
	time.Sleep(50 * time.Millisecond)

	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	cc := w.Header().Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("expected no-cache, got %s", cc)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v -race ./internal/dashboard/`
Expected: FAIL — `New` function and `Dashboard` type not defined.

**Step 3: Write minimal implementation**

```go
// internal/dashboard/dashboard.go
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
func New(metrics *Metrics, routes RouteProvider, version string, startTime time.Time) *Dashboard {
	d := &Dashboard{
		metrics:   metrics,
		routes:    routes,
		version:   version,
		startTime: startTime,
	}

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("dashboard: failed to create sub filesystem: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /events", d.handleEvents)
	mux.HandleFunc("GET /api/routes", d.handleAPIRoutes)
	mux.HandleFunc("GET /api/stats", d.handleAPIStats)
	mux.Handle("GET /", http.FileServerFS(staticSub))

	d.mux = mux
	return d
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
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -race ./internal/dashboard/`
Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/dashboard/dashboard.go internal/dashboard/dashboard_test.go
git commit -m "feat(dashboard): add HTTP handlers for dashboard UI, API, and SSE"
```

---

### Task 4: Wire Dashboard into Daemon

**Files:**
- Modify: `internal/daemon/daemon.go` (Daemon struct, New(), handleRequest())
- Modify: `internal/daemon/daemon_test.go` (add statusCapture tests)

**Step 1: Write the failing tests**

Add to `internal/daemon/daemon_test.go`:

```go
import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusCapture_CapturesWriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	sc.WriteHeader(http.StatusNotFound)

	if sc.status != 404 {
		t.Errorf("expected status 404, got %d", sc.status)
	}
	if w.Code != 404 {
		t.Errorf("expected underlying writer to have 404, got %d", w.Code)
	}
}

func TestStatusCapture_DefaultsToZero(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	if sc.status != 0 {
		t.Errorf("expected initial status 0, got %d", sc.status)
	}
}

func TestStatusCapture_OnlyFirstWriteHeaderCaptured(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	sc.WriteHeader(http.StatusOK)
	sc.WriteHeader(http.StatusNotFound)

	if sc.status != 200 {
		t.Errorf("expected first status 200, got %d", sc.status)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -race ./internal/daemon/ -run TestStatusCapture`
Expected: FAIL — `statusCapture` type not defined.

**Step 3: Implement statusCapture and wire dashboard**

**3a. Add imports to daemon.go** — add `"bufio"` and the dashboard import:

```go
import (
	"bufio"
	// ... existing imports ...
	"github.com/alexcatdad/paw-proxy/internal/dashboard"
)
```

**3b. Add fields to `Daemon` struct** — add after `logFile *os.File`:

```go
	metrics   *dashboard.Metrics
	dash      *dashboard.Dashboard
```

**3c. Initialize dashboard in `New()`** — add before the `return &Daemon{` block:

```go
	metrics := dashboard.NewMetrics(1000)
	dash := dashboard.New(metrics, registry, api.Version, time.Now())
```

And include them in the return:

```go
	return &Daemon{
		config:    config,
		dnsServer: dnsServer,
		registry:  registry,
		apiServer: apiServer,
		certCache: certCache,
		proxy:     proxy.New(),
		logger:    logger,
		logFile:   logFile,
		metrics:   metrics,
		dash:      dash,
	}, nil
```

**3d. Replace `handleRequest()`** with dashboard intercept + metrics recording:

```go
func (d *Daemon) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Dashboard intercept — not recorded in metrics to avoid feedback loop
	if api.ExtractName(r.Host) == "_paw" {
		d.dash.ServeHTTP(w, r)
		return
	}

	start := time.Now()

	route, ok := d.registry.LookupByHost(r.Host)
	if !ok {
		d.serveNotFound(w, r)
		elapsed := time.Since(start).Milliseconds()
		d.logger.Info("request",
			"host", r.Host,
			"method", r.Method,
			"path", r.URL.Path,
			"status", 404,
			"duration_ms", elapsed,
		)
		d.metrics.Record(dashboard.RequestEntry{
			Timestamp:  start,
			Host:       r.Host,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: 404,
			LatencyMs:  elapsed,
		})
		return
	}

	rw := &statusCapture{ResponseWriter: w}
	d.proxy.ServeHTTP(rw, r, route.Upstream)

	status := rw.status
	if status == 0 {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			status = 101
		} else {
			status = 200
		}
	}

	elapsed := time.Since(start).Milliseconds()
	d.logger.Info("request",
		"host", r.Host,
		"method", r.Method,
		"path", r.URL.Path,
		"route", route.Name,
		"upstream", route.Upstream,
		"status", status,
		"duration_ms", elapsed,
	)
	d.metrics.Record(dashboard.RequestEntry{
		Timestamp:  start,
		Host:       r.Host,
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: status,
		LatencyMs:  elapsed,
		Route:      route.Name,
		Upstream:   route.Upstream,
	})
}
```

**3e. Add statusCapture type** at the bottom of daemon.go:

```go
// statusCapture wraps an http.ResponseWriter to capture the status code.
// It forwards Hijack and Flush to the underlying writer so WebSocket
// and SSE proxying continue to work.
type statusCapture struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusCapture) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusCapture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijack not supported")
}

func (s *statusCapture) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusCapture) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -race ./internal/daemon/`
Expected: All PASS.

Then run the full suite:

Run: `go test -v -race ./...`
Expected: All PASS.

**Step 5: Run go vet**

Run: `go vet ./...`
Expected: Clean output.

**Step 6: Commit**

```bash
git add internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "feat(dashboard): wire dashboard into daemon with status capture and metrics recording"
```

---

### Task 5: Integration Test

**Files:**
- Modify: `integration-tests.sh`

**Step 1: Read the current integration test to find the right insertion point**

Run: `cat integration-tests.sh`
Find the section after route tests where dashboard checks should go.

**Step 2: Add dashboard checks**

Add after the existing route/proxy tests:

```bash
# Test dashboard is accessible
echo "Testing dashboard at _paw.test..."
DASH_STATUS=$(curl -sk -o /dev/null -w '%{http_code}' https://_paw.test/)
if [ "$DASH_STATUS" -ne 200 ]; then
  echo "FAIL: Dashboard returned $DASH_STATUS, expected 200"
  exit 1
fi
echo "PASS: Dashboard accessible at _paw.test"

# Test dashboard API
echo "Testing dashboard API..."
ROUTES_JSON=$(curl -sk https://_paw.test/api/routes)
if ! echo "$ROUTES_JSON" | grep -q '\['; then
  echo "FAIL: /api/routes did not return JSON array"
  exit 1
fi
echo "PASS: Dashboard API returns routes"

STATS_JSON=$(curl -sk https://_paw.test/api/stats)
if ! echo "$STATS_JSON" | grep -q '"version"'; then
  echo "FAIL: /api/stats did not return version"
  exit 1
fi
echo "PASS: Dashboard API returns stats"
```

**Step 3: Run integration tests locally (requires daemon running)**

Run: `./integration-tests.sh`
Expected: All tests pass including new dashboard checks.

**Step 4: Commit**

```bash
git add integration-tests.sh
git commit -m "test: add dashboard integration tests for _paw.test"
```

---

## Summary of All Files

| Action | File | Description |
|--------|------|-------------|
| Create | `internal/dashboard/metrics.go` | Ring buffer, per-route counters, SSE subscriber fan-out |
| Create | `internal/dashboard/metrics_test.go` | 8 tests: buffer ops, wraparound, route stats, subscribers, concurrency |
| Create | `internal/dashboard/static/index.html` | Dashboard HTML structure |
| Create | `internal/dashboard/static/style.css` | Dark/light theme styling |
| Create | `internal/dashboard/static/app.js` | SSE client, DOM updates, filtering, route table refresh |
| Create | `internal/dashboard/dashboard.go` | HTTP handlers: file server, SSE, routes API, stats API |
| Create | `internal/dashboard/dashboard_test.go` | 6 tests: HTML/CSS/JS serving, API responses, SSE headers |
| Modify | `internal/daemon/daemon.go` | Add statusCapture, Metrics, Dashboard; intercept `_paw.test`; record metrics |
| Modify | `internal/daemon/daemon_test.go` | 3 tests: statusCapture behavior |
| Modify | `integration-tests.sh` | Dashboard curl checks |

## Verification Checklist

After all tasks:
- [ ] `go test -v -race ./...` — all pass
- [ ] `go vet ./...` — clean
- [ ] `go build ./cmd/paw-proxy` — compiles
- [ ] `go build ./cmd/up` — compiles
- [ ] Visit `https://_paw.test` with daemon running — dashboard loads
- [ ] Start `up bun dev` — route appears in dashboard
- [ ] Request feed shows live requests via SSE
