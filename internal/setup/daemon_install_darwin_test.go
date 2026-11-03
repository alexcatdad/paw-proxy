//go:build darwin

package setup

import (
	"errors"
	"strings"
	"testing"

	"github.com/alexcatdad/paw-proxy/internal/appbundle"
	"github.com/alexcatdad/paw-proxy/internal/desktop"
)

type fakeDeps struct {
	bundle      appbundle.Bundle
	locateErr   error
	reply       desktop.Reply
	sendErr     error
	legacyErr   error
	legacyCalls int
	sent        []desktop.Op
}

func (f *fakeDeps) deps() daemonDeps {
	return daemonDeps{
		locate: func() (appbundle.Bundle, error) { return f.bundle, f.locateErr },
		sendToApp: func(b appbundle.Bundle, op desktop.Op) (desktop.Reply, error) {
			f.sent = append(f.sent, op)
			return f.reply, f.sendErr
		},
		installLegacy: func(*Config) error { f.legacyCalls++; return f.legacyErr },
	}
}

var release = appbundle.Bundle{Path: "/Applications/Paw Proxy.app", ID: appbundle.BundleID, Version: "2026.11.1", Build: "1"}

func TestInstallDaemon(t *testing.T) {
	tests := []struct {
		name       string
		fake       fakeDeps
		legacyFlag bool
		requireApp bool
		wantLegacy int
		wantSent   int
		wantMsg    string
		wantErr    string
	}{
		{name: "bare binary, app not yet required", fake: fakeDeps{locateErr: appbundle.ErrNotInBundle}, wantLegacy: 1, wantMsg: "LaunchAgent installed"},
		{name: "bare binary, app required", fake: fakeDeps{locateErr: appbundle.ErrNotInBundle}, requireApp: true, wantErr: "--legacy-launchagent"},
		{name: "bare binary, app required, flag", fake: fakeDeps{locateErr: appbundle.ErrNotInBundle}, requireApp: true, legacyFlag: true, wantLegacy: 1, wantMsg: "Legacy LaunchAgent"},
		{name: "flag wins inside bundle", fake: fakeDeps{bundle: release}, legacyFlag: true, wantLegacy: 1, wantMsg: "Legacy LaunchAgent"},
		{name: "bundle registered", fake: fakeDeps{bundle: release, reply: desktop.Reply{Outcome: desktop.OutcomeRegistered}}, wantSent: 1, wantMsg: "registered by Paw Proxy.app"},
		{name: "bundle needs approval", fake: fakeDeps{bundle: release, reply: desktop.Reply{Outcome: desktop.OutcomeRequiresApproval}}, wantSent: 1, wantMsg: "Login Items"},
		{name: "bundle rolled back", fake: fakeDeps{bundle: release, reply: desktop.Reply{Outcome: desktop.OutcomeRolledBack, Detail: "verify timeout"}}, wantSent: 1, wantErr: "previous LaunchAgent was restored: verify timeout"},
		{name: "bundle failed", fake: fakeDeps{bundle: release, reply: desktop.Reply{Outcome: desktop.OutcomeFailed, Detail: "boom"}}, wantSent: 1, wantErr: "boom"},
		{name: "app unreachable", fake: fakeDeps{bundle: release, sendErr: errors.New("no reply")}, wantSent: 1, wantErr: "no reply"},
		{name: "dev bundle refused", fake: fakeDeps{bundle: appbundle.Bundle{ID: appbundle.BundleID + ".dev"}}, wantErr: "development builds"},
		{name: "broken bundle", fake: fakeDeps{locateErr: errors.New("bad plist")}, wantErr: "bad plist"},
		{name: "legacy install fails", fake: fakeDeps{locateErr: appbundle.ErrNotInBundle, legacyErr: errors.New("bootstrap failed")}, wantLegacy: 1, wantErr: "bootstrap failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fake
			msg, err := installDaemon(&Config{LegacyLaunchAgent: tt.legacyFlag}, f.deps(), tt.requireApp)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil || !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("msg = %q, err = %v; want %q", msg, err, tt.wantMsg)
			}
			if f.legacyCalls != tt.wantLegacy {
				t.Errorf("legacy installs = %d, want %d", f.legacyCalls, tt.wantLegacy)
			}
			if len(f.sent) != tt.wantSent {
				t.Errorf("app requests = %v, want %d", f.sent, tt.wantSent)
			}
			for _, op := range f.sent {
				if op != desktop.OpRegister {
					t.Errorf("sent %q, want register", op)
				}
			}
		})
	}
}

func TestAppRequiredStaysOffUntilCaskShips(t *testing.T) {
	// Flipping this before the cask exists breaks every formula install.
	if appRequired {
		t.Fatal("appRequired must stay false until the cask is published")
	}
}

func TestUnregisterFromApp(t *testing.T) {
	notInBundle := func() (appbundle.Bundle, error) { return appbundle.Bundle{}, appbundle.ErrNotInBundle }
	inBundle := func() (appbundle.Bundle, error) { return release, nil }
	dev := func() (appbundle.Bundle, error) { return appbundle.Bundle{ID: appbundle.BundleID + ".dev"}, nil }
	reply := func(o desktop.Outcome, err error) func(appbundle.Bundle, desktop.Op) (desktop.Reply, error) {
		return func(_ appbundle.Bundle, op desktop.Op) (desktop.Reply, error) {
			if op != desktop.OpUnregister {
				t.Errorf("sent %q, want unregister", op)
			}
			return desktop.Reply{Outcome: o}, err
		}
	}
	never := func(appbundle.Bundle, desktop.Op) (desktop.Reply, error) {
		t.Error("app contacted outside a release bundle")
		return desktop.Reply{}, nil
	}

	if inApp, err := unregisterFromApp("", notInBundle, never); inApp || err != nil {
		t.Errorf("bare binary: %v, %v", inApp, err)
	}
	if inApp, err := unregisterFromApp("", dev, never); inApp || err != nil {
		t.Errorf("dev bundle: %v, %v", inApp, err)
	}
	for _, o := range []desktop.Outcome{desktop.OutcomeUnregistered, desktop.OutcomeNotRegistered} {
		if inApp, err := unregisterFromApp("", inBundle, reply(o, nil)); !inApp || err != nil {
			t.Errorf("%s: %v, %v", o, inApp, err)
		}
	}
	if _, err := unregisterFromApp("", inBundle, reply(desktop.OutcomeFailed, nil)); err == nil {
		t.Error("failed outcome accepted")
	}
	if _, err := unregisterFromApp("", inBundle, reply("", errors.New("timeout"))); err == nil {
		t.Error("send error swallowed")
	}
}
