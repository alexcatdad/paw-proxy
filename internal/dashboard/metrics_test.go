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
	entries := m.Recent(100)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMetrics_RingBufferWraparound(t *testing.T) {
	m := NewMetrics(3)
	for i := 0; i < 5; i++ {
		m.Record(makeEntry("app", 200, int64(i)))
	}
	entries := m.Recent(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (buffer size), got %d", len(entries))
	}
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
	}
}

func TestMetrics_SlowSubscriberDoesNotBlock(t *testing.T) {
	m := NewMetrics(10)
	_ = m.Subscribe()

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
