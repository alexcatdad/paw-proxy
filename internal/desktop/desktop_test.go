package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type vector struct {
	Name  string `json:"name"`
	JSON  string `json:"json"`
	Valid bool   `json:"valid"`
}

type vectorFile struct {
	ID       string   `json:"id"`
	Requests []vector `json:"requests"`
	Replies  []vector `json:"replies"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	data, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectorFile
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// parseRequest mirrors the validation the app performs, so the shared
// vectors pin the request format from the Go side too.
func parseRequest(data []byte) (Request, error) {
	var r Request
	if err := json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	if r.Version != ProtocolVersion || !idPattern.MatchString(r.ID) || !validOp(r.Op) {
		return r, errors.New("invalid request")
	}
	return r, nil
}

func TestVectors(t *testing.T) {
	v := loadVectors(t)
	for _, c := range v.Requests {
		_, err := parseRequest([]byte(c.JSON))
		if (err == nil) != c.Valid {
			t.Errorf("request %q: err = %v, want valid=%v", c.Name, err, c.Valid)
		}
	}
	for _, c := range v.Replies {
		_, err := ParseReply([]byte(c.JSON), v.ID)
		if (err == nil) != c.Valid {
			t.Errorf("reply %q: err = %v, want valid=%v", c.Name, err, c.Valid)
		}
	}
}

// fakeApp returns an Opener that reads the request and writes a reply,
// as the app would. mutate can alter the reply bytes or file mode.
func fakeApp(t *testing.T, outcome Outcome, mutate func(path string, data []byte) []byte, mode os.FileMode) (Opener, *Request) {
	seen := &Request{}
	return func(ctx context.Context, reqPath string) error {
		data, err := os.ReadFile(reqPath)
		if err != nil {
			return err
		}
		fi, err := os.Stat(reqPath)
		if err != nil {
			return err
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("request mode %#o, want 0600", fi.Mode().Perm())
		}
		req, err := parseRequest(data)
		if err != nil {
			return err
		}
		*seen = req
		reply, _ := json.Marshal(Reply{Version: 1, ID: req.ID, Outcome: outcome, Detail: "d"})
		replyPath := strings.TrimSuffix(reqPath, ".json") + ".reply.json"
		if mutate != nil {
			reply = mutate(replyPath, reply)
		}
		go func() {
			time.Sleep(5 * time.Millisecond) // reply arrives after the first poll
			tmp := replyPath + ".tmp"
			if err := os.WriteFile(tmp, reply, mode); err != nil {
				return
			}
			os.Chmod(tmp, mode)
			os.Rename(tmp, replyPath)
		}()
		return nil
	}, seen
}

func newClient(t *testing.T, open Opener) *Client {
	return &Client{
		Dir:      filepath.Join(t.TempDir(), "requests"),
		Owner:    os.Getuid(),
		Open:     open,
		Timeout:  time.Second,
		Interval: time.Millisecond,
	}
}

func TestSend_Outcomes(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeRegistered, OutcomeRequiresApproval, OutcomeRolledBack, OutcomeFailed, OutcomeUnregistered} {
		t.Run(string(outcome), func(t *testing.T) {
			open, seen := fakeApp(t, outcome, nil, 0o600)
			c := newClient(t, open)
			reply, err := c.Send(context.Background(), OpRegister)
			if err != nil {
				t.Fatal(err)
			}
			if reply.Outcome != outcome || reply.ID != seen.ID || seen.Op != OpRegister {
				t.Errorf("reply %+v for request %+v", reply, seen)
			}
			entries, _ := os.ReadDir(c.Dir)
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".tmp") {
					t.Errorf("leftover file %s", e.Name())
				}
			}
			fi, err := os.Stat(c.Dir)
			if err != nil || fi.Mode().Perm() != 0o700 {
				t.Errorf("request dir mode = %v, %v", fi.Mode().Perm(), err)
			}
		})
	}
}

func TestSend_Timeout(t *testing.T) {
	c := newClient(t, func(context.Context, string) error { return nil })
	c.Timeout = 30 * time.Millisecond
	if _, err := c.Send(context.Background(), OpStatus); err == nil || !strings.Contains(err.Error(), "no reply") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestSend_RejectsBadReplies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string, []byte) []byte
		mode   os.FileMode
		want   string
	}{
		{"world readable", nil, 0o644, "mode"},
		{"wrong id", func(_ string, b []byte) []byte {
			return []byte(`{"version":1,"id":"ffffffffffffffffffffffffffffffff","outcome":"registered"}`)
		}, 0o600, "does not match"},
		{"oversized", func(_ string, b []byte) []byte { return []byte(strings.Repeat(" ", maxFileSize+10)) }, 0o600, "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open, _ := fakeApp(t, OutcomeRegistered, tt.mutate, tt.mode)
			_, err := newClient(t, open).Send(context.Background(), OpRegister)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSend_RejectsSymlinkReply(t *testing.T) {
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	open := func(ctx context.Context, reqPath string) error {
		return os.Symlink(target, strings.TrimSuffix(reqPath, ".json")+".reply.json")
	}
	if _, err := newClient(t, open).Send(context.Background(), OpRegister); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("err = %v, want symlink rejection", err)
	}
}

func TestSend_OpenerFailureAndUnknownOp(t *testing.T) {
	c := newClient(t, func(context.Context, string) error { return errors.New("app not installed") })
	if _, err := c.Send(context.Background(), OpRegister); err == nil || !strings.Contains(err.Error(), "app not installed") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(c.Dir); len(entries) != 0 {
		t.Errorf("request file left behind after opener failure")
	}
	if _, err := c.Send(context.Background(), Op("exec")); err == nil {
		t.Fatal("unknown op accepted")
	}
}
