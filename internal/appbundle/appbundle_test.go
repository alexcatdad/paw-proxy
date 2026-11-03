package appbundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key><string>%s</string>
	<key>CFBundleShortVersionString</key><string>2026.11.1</string>
	<key>LSUIElement</key><true/>
	<key>Nested</key><dict><key>CFBundleVersion</key><string>999</string></dict>
	<key>CFBundleVersion</key><string>7</string>
</dict>
</plist>
`

// makeBundle builds <dir>/Paw Proxy.app with an Info.plist and a helper.
func makeBundle(t *testing.T, dir, id string) (app, helper string) {
	t.Helper()
	app = filepath.Join(dir, "Paw Proxy.app")
	helpers := filepath.Join(app, "Contents", "Helpers")
	if err := os.MkdirAll(helpers, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := []byte(fmt.Sprintf(infoPlist, id))
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), plist, 0o644); err != nil {
		t.Fatal(err)
	}
	helper = filepath.Join(helpers, "paw-proxy")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return app, helper
}

func realDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir()) // macOS TempDir lives under a /var symlink
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFromExecutable_DirectHelper(t *testing.T) {
	app, helper := makeBundle(t, realDir(t), BundleID)
	b, err := FromExecutable(helper)
	if err != nil {
		t.Fatal(err)
	}
	want := Bundle{Path: app, ID: BundleID, Version: "2026.11.1", Build: "7"}
	if b != want {
		t.Errorf("got %+v, want %+v", b, want)
	}
	if b.IsDev() {
		t.Error("release bundle reported as dev")
	}
}

func TestFromExecutable_ThroughSymlinkChain(t *testing.T) {
	dir := realDir(t)
	app, helper := makeBundle(t, dir, BundleID)
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Mimic Homebrew: bin/paw-proxy -> Caskroom link -> helper.
	mid := filepath.Join(dir, "caskroom-paw-proxy")
	if err := os.Symlink(helper, mid); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "paw-proxy")
	if err := os.Symlink(mid, link); err != nil {
		t.Fatal(err)
	}
	b, err := FromExecutable(link)
	if err != nil {
		t.Fatal(err)
	}
	if b.Path != app {
		t.Errorf("Path = %q, want %q", b.Path, app)
	}
}

func TestFromExecutable_DevBundle(t *testing.T) {
	_, helper := makeBundle(t, realDir(t), BundleID+".dev")
	b, err := FromExecutable(helper)
	if err != nil {
		t.Fatal(err)
	}
	if !b.IsDev() {
		t.Error("dev bundle not reported as dev")
	}
}

func TestFromExecutable_NotInBundle(t *testing.T) {
	dir := realDir(t)
	exe := filepath.Join(dir, "paw-proxy")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := FromExecutable(exe); !errors.Is(err, ErrNotInBundle) {
		t.Errorf("err = %v, want ErrNotInBundle", err)
	}

	// Contents/MacOS is the app's own executable, not a helper.
	macos := filepath.Join(dir, "X.app", "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(macos, "X")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := FromExecutable(exe); !errors.Is(err, ErrNotInBundle) {
		t.Errorf("MacOS executable: err = %v, want ErrNotInBundle", err)
	}
}

func TestFromExecutable_BadPlist(t *testing.T) {
	app, helper := makeBundle(t, realDir(t), BundleID)
	plist := filepath.Join(app, "Contents", "Info.plist")

	if err := os.WriteFile(plist, []byte("<plist><dict><key>x</key>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FromExecutable(helper); err == nil {
		t.Error("truncated plist accepted")
	}

	if err := os.WriteFile(plist, []byte(`<plist><dict><key>CFBundleName</key><string>x</string></dict></plist>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FromExecutable(helper); err == nil {
		t.Error("plist without CFBundleIdentifier accepted")
	}

	if err := os.Remove(plist); err != nil {
		t.Fatal(err)
	}
	if _, err := FromExecutable(helper); err == nil {
		t.Error("missing plist accepted")
	}
}
