package tailnet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const host = "mac.example.ts.net"

var port = strconv.Itoa(Port("progress"))

// fakeCLI answers `status --json` and `serve status --json` from its fields
// and records every other call.
type fakeCLI struct {
	status string
	serve  string
	fail   string // substring of a command line that should fail
	calls  []string
}

func (f *fakeCLI) run(_ context.Context, args ...string) ([]byte, error) {
	line := strings.Join(args, " ")
	if f.fail != "" && strings.Contains(line, f.fail) {
		return nil, errors.New("exit status 1")
	}
	switch line {
	case "status --json":
		return []byte(f.status), nil
	case "serve status --json":
		return []byte(f.serve), nil
	}
	f.calls = append(f.calls, line)
	return nil, nil
}

const running = `{"BackendState":"Running","Self":{"DNSName":"mac.example.ts.net."},"CertDomains":["mac.example.ts.net"]}`

func newExposer(t *testing.T, cli *fakeCLI, alive bool) *Exposer {
	t.Helper()
	e := New(filepath.Join(t.TempDir(), "tailnet.json"))
	e.run = cli.run
	e.alive = func(string) bool { return alive }
	return e
}

func webConfig(port, proxy string) string {
	return `{"TCP":{"` + port + `":{"HTTPS":true}},"Web":{"` + host + `:` + port + `":{"Handlers":{"/":{"Proxy":"` + proxy + `"}}}}}`
}

func TestPortIsStableAndInRange(t *testing.T) {
	for _, name := range []string{"a", "progress", "frontend.shop", strings.Repeat("x", 63)} {
		p := Port(name)
		if p < 20000 || p > 29999 {
			t.Errorf("Port(%q) = %d, outside 20000-29999", name, p)
		}
		if p != Port(name) {
			t.Errorf("Port(%q) is not stable", name)
		}
	}
}

func TestExposeServesAndRecords(t *testing.T) {
	cli := &fakeCLI{status: running, serve: `{}`}
	e := newExposer(t, cli, false)

	got, err := e.Expose(context.Background(), "progress", "localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	p := strconv.Itoa(Port("progress"))
	if got != "https://"+host+":"+p {
		t.Fatalf("unexpected URL %q", got)
	}
	want := "serve --bg --yes --https=" + p + " http://127.0.0.1:3000"
	if len(cli.calls) != 1 || cli.calls[0] != want {
		t.Fatalf("calls = %q, want %q", cli.calls, want)
	}
	data, err := os.ReadFile(e.ledger)
	if err != nil || !strings.Contains(string(data), `"`+p+`":"http://127.0.0.1:3000"`) {
		t.Fatalf("ledger = %q, %v", data, err)
	}
}

func TestExposeRefusals(t *testing.T) {
	tests := []struct {
		name   string
		cli    fakeCLI
		alive  bool
		reason string
	}{
		{"cli missing", fakeCLI{fail: "status --json"}, false, "not available"},
		{"stopped", fakeCLI{status: `{"BackendState":"Stopped","Self":{"DNSName":"mac.example.ts.net."}}`}, false, "not running"},
		{"no https", fakeCLI{status: `{"BackendState":"Running","Self":{"DNSName":"mac.example.ts.net."}}`}, false, "certificates"},
		{"funnel", fakeCLI{status: running, serve: `{"AllowFunnel":{"` + host + `:` + port + `":true}}`}, false, "funnel"},
		{"raw tcp", fakeCLI{status: running, serve: `{"TCP":{"` + port + `":{"TCPForward":"127.0.0.1:1"}}}`}, false, "in use"},
		{"live proxy", fakeCLI{status: running, serve: webConfig(port, "http://127.0.0.1:9")}, true, "in use"},
		{"file handler", fakeCLI{status: running, serve: `{"Web":{"` + host + `:` + port + `":{"Handlers":{"/":{"Path":"/tmp"}}}}}`}, false, "in use"},
		{"serve fails", fakeCLI{status: running, serve: `{}`, fail: "serve --bg"}, false, "serve failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newExposer(t, &tt.cli, tt.alive)
			_, err := e.Expose(context.Background(), "progress", "localhost:3000")
			if err == nil || !strings.Contains(err.Error(), tt.reason) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.reason)
			}
			if len(tt.cli.calls) != 0 {
				t.Fatalf("unexpected calls %q", tt.cli.calls)
			}
		})
	}
}

func TestExposeReplacesDeadLeftover(t *testing.T) {
	cli := &fakeCLI{status: running, serve: webConfig(port, "http://127.0.0.1:9")}
	e := newExposer(t, cli, false)
	if _, err := e.Expose(context.Background(), "progress", "localhost:3000"); err != nil {
		t.Fatal(err)
	}
	if len(cli.calls) != 1 {
		t.Fatalf("calls = %q", cli.calls)
	}
}

func TestUnexposeOnlyRemovesUnchangedOwnRoute(t *testing.T) {
	off := "serve --yes --https=" + port + " off"

	t.Run("unchanged", func(t *testing.T) {
		cli := &fakeCLI{status: running, serve: `{}`}
		e := newExposer(t, cli, false)
		if _, err := e.Expose(context.Background(), "progress", "localhost:3000"); err != nil {
			t.Fatal(err)
		}
		cli.serve = webConfig(port, "http://127.0.0.1:3000")
		e.Unexpose(context.Background(), "progress")
		if cli.calls[len(cli.calls)-1] != off {
			t.Fatalf("calls = %q, want last %q", cli.calls, off)
		}
		if len(e.served) != 0 {
			t.Fatalf("ledger not cleared: %v", e.served)
		}
	})

	t.Run("repurposed", func(t *testing.T) {
		cli := &fakeCLI{status: running, serve: `{}`}
		e := newExposer(t, cli, false)
		if _, err := e.Expose(context.Background(), "progress", "localhost:3000"); err != nil {
			t.Fatal(err)
		}
		cli.serve = webConfig(port, "http://127.0.0.1:4000")
		e.Unexpose(context.Background(), "progress")
		if len(cli.calls) != 1 || len(e.served) != 0 {
			t.Fatalf("calls = %q, ledger = %v", cli.calls, e.served)
		}
	})

	t.Run("never exposed", func(t *testing.T) {
		cli := &fakeCLI{status: running, serve: webConfig(port, "http://127.0.0.1:3000")}
		e := newExposer(t, cli, false)
		e.Unexpose(context.Background(), "progress")
		if len(cli.calls) != 0 {
			t.Fatalf("calls = %q", cli.calls)
		}
	})
}

func TestResetSweepsLedgerFromEarlierRun(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "tailnet.json")
	if err := os.WriteFile(ledger, []byte(`{"`+port+`":"http://127.0.0.1:3000"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cli := &fakeCLI{status: running, serve: webConfig(port, "http://127.0.0.1:3000")}
	e := New(ledger)
	e.run = cli.run

	e.Reset(context.Background())
	if len(cli.calls) != 1 || !strings.HasSuffix(cli.calls[0], " off") {
		t.Fatalf("calls = %q", cli.calls)
	}
	if data, _ := os.ReadFile(ledger); string(data) != "{}" {
		t.Fatalf("ledger = %q", data)
	}
}

func TestResetKeepsEntryWhenTailscaleIsDown(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "tailnet.json")
	if err := os.WriteFile(ledger, []byte(`{"20001":"http://127.0.0.1:3000"}`), 0600); err != nil {
		t.Fatal(err)
	}
	e := New(ledger)
	e.run = (&fakeCLI{fail: "status --json"}).run
	e.Reset(context.Background())
	if len(e.served) != 1 {
		t.Fatalf("ledger = %v, want the entry kept for a later retry", e.served)
	}
}

func TestListening(t *testing.T) {
	if listening("http://127.0.0.1:1") {
		t.Error("port 1 should not be listening")
	}
	if !listening("not a url") {
		t.Error("an unparseable target must count as in use")
	}
}
