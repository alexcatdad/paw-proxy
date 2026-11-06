// Package desktop is the CLI side of the one-shot request/reply exchange with
// Paw Proxy.app. launchd registration must happen inside the app process so
// macOS attributes the background item to Paw Proxy, so the CLI writes a
// private request file, launches the app through Launch Services, and waits
// for a private reply file. There is no listener or broker.
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// ProtocolVersion is the request/reply file format version.
const ProtocolVersion = 1

// maxFileSize bounds request and reply files.
const maxFileSize = 64 << 10

// Op is a request operation.
type Op string

const (
	OpRegister   Op = "register"   // register the daemon agent, migrating dev.paw-proxy if present
	OpUnregister Op = "unregister" // unregister the daemon agent and the menu-bar login item
	OpStatus     Op = "status"     // report registration status without changing anything
)

// Outcome is the result reported by the app.
type Outcome string

const (
	OutcomeRegistered       Outcome = "registered"
	OutcomeRequiresApproval Outcome = "requiresApproval"
	OutcomeUnregistered     Outcome = "unregistered"
	OutcomeNotRegistered    Outcome = "notRegistered"
	OutcomeRolledBack       Outcome = "rolledBack" // migration failed; legacy agent restored
	OutcomeFailed           Outcome = "failed"
)

// Request is written by the CLI.
type Request struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Op      Op     `json:"op"`
}

// Reply is written by the app.
type Reply struct {
	Version int     `json:"version"`
	ID      string  `json:"id"`
	Outcome Outcome `json:"outcome"`
	Detail  string  `json:"detail,omitempty"`
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Opener launches the app with a request file. Production code uses
// LaunchServicesOpener; tests inject fakes.
type Opener func(ctx context.Context, requestPath string) error

// Client sends requests to the app.
type Client struct {
	Dir      string                  // request directory, e.g. <support>/requests
	Owner    int                     // UID that must own request/reply files (the real user)
	Open     Opener                  // launches the app
	Chown    func(path string) error // hands created files to Owner when running as root; may be nil
	Timeout  time.Duration           // overall wait for the reply
	Interval time.Duration           // reply poll interval
}

// Send writes a request, launches the app and waits for its reply. The
// request and reply files are removed before returning.
func (c *Client) Send(ctx context.Context, op Op) (Reply, error) {
	if !validOp(op) {
		return Reply{}, fmt.Errorf("unknown op %q", op)
	}
	id, err := newID()
	if err != nil {
		return Reply{}, err
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return Reply{}, fmt.Errorf("creating request dir: %w", err)
	}
	if err := os.Chmod(c.Dir, 0o700); err != nil { //nolint:gosec // G302: a directory needs the execute bit; 0700 is owner-only
		return Reply{}, err
	}
	if err := c.chown(c.Dir); err != nil {
		return Reply{}, err
	}

	reqPath := filepath.Join(c.Dir, id+".json")
	replyPath := filepath.Join(c.Dir, id+".reply.json")
	defer func() { _ = os.Remove(reqPath) }()
	defer func() { _ = os.Remove(replyPath) }()

	data, err := json.Marshal(Request{Version: ProtocolVersion, ID: id, Op: op})
	if err != nil {
		return Reply{}, err
	}
	if err := writePrivate(reqPath, data); err != nil {
		return Reply{}, fmt.Errorf("writing request: %w", err)
	}
	if err := c.chown(reqPath); err != nil {
		return Reply{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if err := c.Open(ctx, reqPath); err != nil {
		return Reply{}, fmt.Errorf("launching Paw Proxy.app: %w", err)
	}

	tick := time.NewTicker(c.Interval)
	defer tick.Stop()
	for {
		reply, err := readReply(replyPath, id, c.Owner)
		if err == nil {
			return reply, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Reply{}, err
		}
		select {
		case <-ctx.Done():
			return Reply{}, fmt.Errorf("no reply from Paw Proxy.app within %s", c.Timeout)
		case <-tick.C:
		}
	}
}

func (c *Client) chown(path string) error {
	if c.Chown == nil {
		return nil
	}
	if err := c.Chown(path); err != nil {
		return fmt.Errorf("handing %s to the user: %w", path, err)
	}
	return nil
}

func validOp(op Op) bool {
	switch op {
	case OpRegister, OpUnregister, OpStatus:
		return true
	}
	return false
}

func validOutcome(o Outcome) bool {
	switch o {
	case OutcomeRegistered, OutcomeRequiresApproval, OutcomeUnregistered,
		OutcomeNotRegistered, OutcomeRolledBack, OutcomeFailed:
		return true
	}
	return false
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// writePrivate creates path exclusively with mode 0600.
func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// readReply returns os.ErrNotExist while the reply has not been written.
// A reply that exists but is not a private regular file owned by owner, is
// oversized, malformed, or answers a different request is an error.
func readReply(path, id string, owner int) (Reply, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return Reply{}, err
	}
	if err := checkPrivateFile(fi, owner); err != nil {
		return Reply{}, fmt.Errorf("reply file: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return Reply{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return Reply{}, err
	}
	if len(data) == 0 {
		// The app writes atomically (temp file + rename), but tolerate a
		// zero-length file as "not yet written".
		return Reply{}, os.ErrNotExist
	}
	return ParseReply(data, id)
}

// ParseReply validates a reply payload for request id.
func ParseReply(data []byte, id string) (Reply, error) {
	if len(data) > maxFileSize {
		return Reply{}, errors.New("reply too large")
	}
	var r Reply
	if err := json.Unmarshal(data, &r); err != nil {
		return Reply{}, fmt.Errorf("malformed reply: %w", err)
	}
	if r.Version != ProtocolVersion {
		return Reply{}, fmt.Errorf("unsupported reply version %d", r.Version)
	}
	if !idPattern.MatchString(r.ID) || r.ID != id {
		return Reply{}, errors.New("reply does not match the request")
	}
	if !validOutcome(r.Outcome) {
		return Reply{}, fmt.Errorf("unknown outcome %q", r.Outcome)
	}
	return r, nil
}
