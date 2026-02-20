package setup

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
)

// resolveRealUID returns the UID of the real user. When running under sudo,
// this is the SUDO_USER's UID, not root's.
func resolveRealUID() (int, error) {
	uid := os.Getuid()
	if uid == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			u, err := user.Lookup(sudoUser)
			if err != nil {
				return 0, fmt.Errorf("looking up SUDO_USER %q: %w", sudoUser, err)
			}
			parsed, err := strconv.Atoi(u.Uid)
			if err != nil {
				return 0, fmt.Errorf("parsing UID for %q: %w", sudoUser, err)
			}
			return parsed, nil
		}
	}
	return uid, nil
}

// chownToRealUser changes ownership of paths to the real (non-root) user when
// running under sudo. This is a no-op when not running as root or when root
// was not reached via sudo (e.g. CI).
func chownToRealUser(paths ...string) error {
	if os.Getuid() != 0 {
		return nil
	}
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return nil
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return fmt.Errorf("looking up SUDO_USER %q: %w", sudoUser, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("parsing UID for %q: %w", sudoUser, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("parsing GID for %q: %w", sudoUser, err)
	}
	for _, p := range paths {
		if err := os.Chown(p, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", p, err)
		}
	}
	return nil
}
