//go:build darwin

package desktop

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// LaunchServicesOpener launches the app at appPath in the background with
// --request. -n starts a fresh instance even when the menu-bar app is
// already running (open drops --args for a running app); that instance
// handles the request without showing UI and exits. -a with the exact path
// avoids Launch Services picking a different copy with the same bundle ID.
// When the CLI runs as root under sudo, the app is launched in the real
// user's GUI session (launchctl asuser + sudo -u) so the app — not root —
// owns the registration.
func LaunchServicesOpener(appPath string, uid int, user string, asRoot bool) Opener {
	return func(ctx context.Context, requestPath string) error {
		args := []string{"-n", "-g", "-a", appPath, "--args", "--request", requestPath}
		var cmd *exec.Cmd
		if asRoot {
			full := append([]string{"asuser", strconv.Itoa(uid), "sudo", "-u", user, "/usr/bin/open"}, args...)
			cmd = exec.CommandContext(ctx, "/bin/launchctl", full...)
		} else {
			cmd = exec.CommandContext(ctx, "/usr/bin/open", args...)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
		return nil
	}
}
