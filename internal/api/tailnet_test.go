package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeTailnet struct {
	err       error
	exposed   []string
	unexposed []string
}

func (f *fakeTailnet) Expose(_ context.Context, name, upstream string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.exposed = append(f.exposed, name+"="+upstream)
	return "https://mac.example.ts.net:20001", nil
}

func (f *fakeTailnet) Unexpose(_ context.Context, name string) {
	f.unexposed = append(f.unexposed, name)
}

func serve(s *Server, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

const tailnetRoute = `{"name":"progress","upstream":"localhost:3000","dir":"/tmp/p","tailnet":true}`

func TestRegisterWithTailnet(t *testing.T) {
	registry := NewRouteRegistry(30 * time.Second)
	s := NewServer("", registry)
	tn := &fakeTailnet{}
	s.SetTailnet(tn)

	rec := serve(s, "POST", "/routes", tailnetRoute)
	var resp RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q, err %v", rec.Code, rec.Body, err)
	}
	if resp.TailnetURL != "https://mac.example.ts.net:20001" || resp.TailnetError != "" {
		t.Fatalf("response = %+v", resp)
	}
	if route, _ := registry.Lookup("progress"); route.TailnetURL != resp.TailnetURL {
		t.Fatalf("route.TailnetURL = %q", route.TailnetURL)
	}
	if len(tn.exposed) != 1 || tn.exposed[0] != "progress=localhost:3000" {
		t.Fatalf("exposed = %q", tn.exposed)
	}

	if rec := serve(s, "DELETE", "/routes/progress", ""); rec.Code != http.StatusOK {
		t.Fatalf("deregister status %d", rec.Code)
	}
	if len(tn.unexposed) != 1 || tn.unexposed[0] != "progress" {
		t.Fatalf("unexposed = %q", tn.unexposed)
	}
}

func TestRegisterFallsBackWhenTailnetFails(t *testing.T) {
	for name, tn := range map[string]Tailnet{"failing": &fakeTailnet{err: errors.New("tailscale is not running")}, "absent": nil} {
		t.Run(name, func(t *testing.T) {
			registry := NewRouteRegistry(30 * time.Second)
			s := NewServer("", registry)
			if tn != nil {
				s.SetTailnet(tn)
			}

			rec := serve(s, "POST", "/routes", tailnetRoute)
			var resp RegisterResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("status %d, body %q, err %v", rec.Code, rec.Body, err)
			}
			if resp.TailnetURL != "" || resp.TailnetError == "" {
				t.Fatalf("response = %+v", resp)
			}
			route, ok := registry.Lookup("progress")
			if !ok || route.TailnetURL != "" {
				t.Fatalf("route = %+v, registered = %v", route, ok)
			}
		})
	}
}

func TestPlainRegistrationNeverTouchesTailnet(t *testing.T) {
	s := NewServer("", NewRouteRegistry(30*time.Second))
	tn := &fakeTailnet{}
	s.SetTailnet(tn)

	rec := serve(s, "POST", "/routes", `{"name":"app","upstream":"localhost:3000","dir":"/tmp/p"}`)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body)
	}
	serve(s, "DELETE", "/routes/app", "")
	if len(tn.exposed)+len(tn.unexposed) != 0 {
		t.Fatalf("exposed %q, unexposed %q", tn.exposed, tn.unexposed)
	}
}

func TestCleanupReturnsExpiredRoutes(t *testing.T) {
	registry := NewRouteRegistry(time.Millisecond)
	if err := registry.Register("progress", "localhost:3000", "/tmp/p"); err != nil {
		t.Fatal(err)
	}
	registry.SetTailnetURL("progress", "https://mac.example.ts.net:20001")
	time.Sleep(5 * time.Millisecond)

	removed := registry.Cleanup()
	if len(removed) != 1 || removed[0].TailnetURL == "" {
		t.Fatalf("removed = %+v", removed)
	}
	if registry.Cleanup() != nil {
		t.Fatal("second cleanup should remove nothing")
	}
}
