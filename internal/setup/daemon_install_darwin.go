//go:build darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/appbundle"
	"github.com/alexcatdad/paw-proxy/internal/desktop"
)

// appRequired makes macOS setup refuse to write the legacy LaunchAgent from
// a binary that is not inside Paw Proxy.app (unless --legacy-launchagent is
// passed). macOS installs come from the cask, whose paw-proxy lives inside
// the app; bare binaries are for development only.
const appRequired = true

// daemonDeps holds the side effects of installing the daemon, so the
// decision logic can be tested without root, launchctl or the app.
type daemonDeps struct {
	locate        func() (appbundle.Bundle, error)
	sendToApp     func(b appbundle.Bundle, op desktop.Op) (desktop.Reply, error)
	installLegacy func(*Config) error
}

func defaultDaemonDeps(config *Config) daemonDeps {
	return daemonDeps{
		locate: appbundle.Locate,
		sendToApp: func(b appbundle.Bundle, op desktop.Op) (desktop.Reply, error) {
			return sendToApp(config.SupportDir, b, op)
		},
		installLegacy: installLaunchAgent,
	}
}

// sendToApp delivers a one-shot request to the app at b.Path.
func sendToApp(supportDir string, b appbundle.Bundle, op desktop.Op) (desktop.Reply, error) {
	uid, err := resolveRealUID()
	if err != nil {
		return desktop.Reply{}, err
	}
	asRoot := os.Getuid() == 0
	client := &desktop.Client{
		Dir:      filepath.Join(supportDir, "requests"),
		Owner:    uid,
		Open:     desktop.LaunchServicesOpener(b.Path, uid, os.Getenv("SUDO_USER"), asRoot),
		Chown:    func(p string) error { return chownToRealUser(p) },
		Timeout:  60 * time.Second, // leaves room for a Login Items approval prompt
		Interval: 200 * time.Millisecond,
	}
	return client.Send(context.Background(), op)
}

// installDaemon chooses between registration through Paw Proxy.app and the
// legacy CLI-written LaunchAgent, and returns a one-line success message.
func installDaemon(config *Config, deps daemonDeps, requireApp bool) (string, error) {
	if config.LegacyLaunchAgent {
		if err := deps.installLegacy(config); err != nil {
			return "", fmt.Errorf("installing LaunchAgent: %w", err)
		}
		return "Legacy LaunchAgent installed and started", nil
	}

	b, err := deps.locate()
	switch {
	case errors.Is(err, appbundle.ErrNotInBundle):
		if requireApp {
			return "", errors.New("this paw-proxy binary is not part of Paw Proxy.app. " +
				"Install the app (brew install --cask alexcatdad/tap/paw-proxy) and run its paw-proxy, " +
				"or pass --legacy-launchagent")
		}
		if err := deps.installLegacy(config); err != nil {
			return "", fmt.Errorf("installing LaunchAgent: %w", err)
		}
		return "LaunchAgent installed and started", nil
	case err != nil:
		return "", fmt.Errorf("inspecting the app bundle: %w", err)
	}

	if b.IsDev() {
		return "", errors.New("development builds of Paw Proxy.app never register the daemon; " +
			"use the installed app or pass --legacy-launchagent")
	}

	reply, err := deps.sendToApp(b, desktop.OpRegister)
	if err != nil {
		return "", fmt.Errorf("registering the daemon through Paw Proxy.app: %w", err)
	}
	switch reply.Outcome {
	case desktop.OutcomeRegistered:
		return "Daemon registered by Paw Proxy.app", nil
	case desktop.OutcomeRequiresApproval:
		return "Daemon registered; approve Paw Proxy in System Settings › General › Login Items to start it", nil
	case desktop.OutcomeRolledBack:
		return "", fmt.Errorf("registration failed and the previous LaunchAgent was restored: %s", reply.Detail)
	default:
		return "", fmt.Errorf("registration failed (%s): %s", reply.Outcome, reply.Detail)
	}
}

// unregisterFromApp asks Paw Proxy.app to unregister the daemon and the
// menu-bar login item. It reports whether the binary runs from a release
// bundle at all; when it does not, there is nothing to unregister.
func unregisterFromApp(supportDir string, locate func() (appbundle.Bundle, error),
	send func(appbundle.Bundle, desktop.Op) (desktop.Reply, error)) (bool, error) {
	b, err := locate()
	if err != nil || b.IsDev() {
		return false, nil
	}
	reply, err := send(b, desktop.OpUnregister)
	if err != nil {
		return true, err
	}
	switch reply.Outcome {
	case desktop.OutcomeUnregistered, desktop.OutcomeNotRegistered:
		return true, nil
	default:
		return true, fmt.Errorf("unregister failed (%s): %s", reply.Outcome, reply.Detail)
	}
}
