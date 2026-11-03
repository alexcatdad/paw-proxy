// Package appbundle locates the Paw Proxy macOS app bundle that contains the
// running binary. The cask links paw-proxy and up from
// "Paw Proxy.app/Contents/Helpers/" onto PATH, so discovery resolves symlinks
// first.
package appbundle

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// BundleID is the installed app's bundle identifier.
	BundleID = "com.alexcatdad.paw-proxy"
	// DaemonLabel is the launchd label of the app-owned daemon agent.
	DaemonLabel = "com.alexcatdad.paw-proxy.daemon"
	// LegacyLabel is the label of the CLI-written LaunchAgent the app replaces.
	LegacyLabel = "dev.paw-proxy"

	maxInfoPlistSize = 1 << 20
)

// ErrNotInBundle reports that the executable is not inside an app bundle's
// Contents/Helpers directory.
var ErrNotInBundle = errors.New("not running from the Paw Proxy app bundle")

// Bundle describes the app bundle that contains the running binary.
type Bundle struct {
	Path    string // absolute path to the .app directory
	ID      string // CFBundleIdentifier
	Version string // CFBundleShortVersionString
	Build   string // CFBundleVersion
}

// IsDev reports whether this is a development build, which must never
// register launchd services.
func (b Bundle) IsDev() bool { return strings.HasSuffix(b.ID, ".dev") }

// Locate finds the bundle containing the current executable.
func Locate() (Bundle, error) {
	exe, err := os.Executable()
	if err != nil {
		return Bundle{}, fmt.Errorf("finding executable: %w", err)
	}
	return FromExecutable(exe)
}

// FromExecutable finds the bundle containing exe, following symlinks.
func FromExecutable(exe string) (Bundle, error) {
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return Bundle{}, fmt.Errorf("resolving %s: %w", exe, err)
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return Bundle{}, err
	}

	helpers := filepath.Dir(real)
	contents := filepath.Dir(helpers)
	app := filepath.Dir(contents)
	if filepath.Base(helpers) != "Helpers" || filepath.Base(contents) != "Contents" || filepath.Ext(app) != ".app" {
		return Bundle{}, ErrNotInBundle
	}

	info, err := readInfoPlist(filepath.Join(contents, "Info.plist"))
	if err != nil {
		return Bundle{}, err
	}
	b := Bundle{
		Path:    app,
		ID:      info["CFBundleIdentifier"],
		Version: info["CFBundleShortVersionString"],
		Build:   info["CFBundleVersion"],
	}
	if b.ID == "" {
		return Bundle{}, fmt.Errorf("%s: Info.plist has no CFBundleIdentifier", app)
	}
	return b, nil
}

// readInfoPlist returns the top-level string values of an Info.plist.
// Binary plists are converted with plutil (macOS only).
func readInfoPlist(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading Info.plist: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxInfoPlistSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading Info.plist: %w", err)
	}
	if len(data) > maxInfoPlistSize {
		return nil, fmt.Errorf("%s: Info.plist too large", path)
	}
	if bytes.HasPrefix(data, []byte("bplist")) {
		data, err = exec.Command("/usr/bin/plutil", "-convert", "xml1", "-o", "-", path).Output()
		if err != nil {
			return nil, fmt.Errorf("converting binary Info.plist: %w", err)
		}
	}
	return parsePlistStrings(data)
}

// parsePlistStrings extracts <key>/<string> pairs from the top-level dict of
// an XML plist. Nested dicts and non-string values are skipped.
func parsePlistStrings(data []byte) (map[string]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	out := map[string]string{}
	depth := 0 // dict nesting depth
	var key string
	var sawDict bool
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing Info.plist: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			if end, ok := tok.(xml.EndElement); ok && end.Name.Local == "dict" {
				depth--
			}
			continue
		}
		switch start.Name.Local {
		case "dict":
			depth++
			sawDict = true
		case "key":
			var k string
			if err := dec.DecodeElement(&k, &start); err != nil {
				return nil, fmt.Errorf("parsing Info.plist: %w", err)
			}
			if depth == 1 {
				key = k
			}
		case "string":
			var v string
			if err := dec.DecodeElement(&v, &start); err != nil {
				return nil, fmt.Errorf("parsing Info.plist: %w", err)
			}
			if depth == 1 && key != "" {
				out[key] = v
			}
			key = ""
		default:
			if depth == 1 {
				key = ""
			}
		}
	}
	if !sawDict {
		return nil, errors.New("parsing Info.plist: no top-level dict")
	}
	return out, nil
}
