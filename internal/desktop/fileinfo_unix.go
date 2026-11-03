//go:build unix

package desktop

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

func checkPrivateFile(fi fs.FileInfo, owner int) error {
	if !fi.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		return fmt.Errorf("mode %#o, want 0600", perm)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot read owner")
	}
	if int(st.Uid) != owner {
		return fmt.Errorf("owned by uid %d, want %d", st.Uid, owner)
	}
	return nil
}
