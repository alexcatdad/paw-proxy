package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func unixHostClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()

	parsed, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", parsed.Host)
			},
		},
		Timeout: 2 * time.Second,
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MyApp", "myapp"},
		{"@scope/pkg", "scope-pkg"},
		{"---", "app"},
		{"", "app"},
		{"UPPER", "upper"},
		{"my-app", "my-app"},
		{"my_app", "my-app"},
		{"Hello World", "hello-world"},
		{"123", "app-123"},
		{"a", "a"},
		{"My.App.Name", "my-app-name"},
		{"--leading-trailing--", "leading-trailing"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetermineNameSanitizesExplicitName(t *testing.T) {
	got := determineName("My Cool App")
	if got != "my-cool-app" {
		t.Fatalf("determineName() = %q, want %q", got, "my-cool-app")
	}
}

func TestHeartbeatReRegistersRouteOnNotFound(t *testing.T) {
	var heartbeatCount atomic.Int32
	var registerCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/routes/myapp/heartbeat":
			if heartbeatCount.Add(1) == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/routes":
			registerCount.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := unixHostClient(t, server)
	state := newRouteState("myapp", "/tmp/project")
	state.SetUpstream("localhost:3000")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go heartbeatWithInterval(ctx, client, state, 20*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registerCount.Load() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected re-registration after heartbeat 404, register calls=%d", registerCount.Load())
}

func TestDeregisterRouteStatusHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/routes/myapp":
			w.WriteHeader(http.StatusInternalServerError)
		case "/routes/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := unixHostClient(t, server)

	if err := deregisterRoute(client, "missing"); err != nil {
		t.Fatalf("expected 404 to be tolerated, got %v", err)
	}

	err := deregisterRoute(client, "myapp")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestRegisterWithFallback(t *testing.T) {
	t.Run("no conflict registers with original name", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		name, err := registerWithFallback(client, "myapp", "localhost:3000", "/tmp/myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "myapp" {
			t.Errorf("got name %q, want %q", name, "myapp")
		}
	})

	t.Run("conflict falls back to directory name", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)

			if body["name"] == "myapp" {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]string{
					"existingDir": "/home/user/myapp",
				})
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		// Dir basename "myapp-worktree" differs from name "myapp", so fallback kicks in
		name, err := registerWithFallback(client, "myapp", "localhost:3000", "/tmp/myapp-worktree")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "myapp-worktree" {
			t.Errorf("got name %q, want %q", name, "myapp-worktree")
		}
	})

	t.Run("conflict with same directory name returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"existingDir": "/other/myapp",
			})
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		// Dir basename "myapp" matches name "myapp" — no fallback possible
		_, err := registerWithFallback(client, "myapp", "localhost:3000", "/tmp/myapp")
		if err == nil {
			t.Fatal("expected error when directory name matches original name")
		}
	})

	t.Run("conflict fallback also conflicts returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"existingDir": "/somewhere/else",
			})
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		// Both "myapp" and fallback "myapp-worktree" get 409
		_, err := registerWithFallback(client, "myapp", "localhost:3000", "/tmp/myapp-worktree")
		if err == nil {
			t.Fatal("expected error when fallback registration also fails")
		}
	})

	t.Run("non-conflict error is returned directly", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "internal server error",
			})
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		_, err := registerWithFallback(client, "myapp", "localhost:3000", "/tmp/myapp-worktree")
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
	})
}

func TestExtractConflictDir(t *testing.T) {
	t.Run("conflict error returns dir", func(t *testing.T) {
		err := &conflictError{dir: "/home/user/project"}
		got := extractConflictDir(err)
		if got != "/home/user/project" {
			t.Errorf("extractConflictDir() = %q, want %q", got, "/home/user/project")
		}
	})

	t.Run("wrapped conflict error returns dir", func(t *testing.T) {
		err := fmt.Errorf("registration failed: %w", &conflictError{dir: "/tmp/app"})
		got := extractConflictDir(err)
		if got != "/tmp/app" {
			t.Errorf("extractConflictDir() = %q, want %q", got, "/tmp/app")
		}
	})

	t.Run("non-conflict error returns empty", func(t *testing.T) {
		err := errors.New("some other error")
		got := extractConflictDir(err)
		if got != "" {
			t.Errorf("extractConflictDir() = %q, want %q", got, "")
		}
	})

	t.Run("nil error returns empty", func(t *testing.T) {
		got := extractConflictDir(nil)
		if got != "" {
			t.Errorf("extractConflictDir(nil) = %q, want %q", got, "")
		}
	})
}
