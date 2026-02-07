//go:build darwin && cgo

// internal/launchd/launchd_darwin_cgo.go
package launchd

/*
#include <launch.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"net"
	"os"
	"unsafe"
)

// Error codes returned by launch_activate_socket as its return value
// (not via errno). These match standard errno constants.
const (
	esrch  = 3 // not launched by launchd
	enoent = 2 // socket name not found in plist
)

// ActivateSocket asks launchd for a pre-bound socket by name.
// Returns (listener, true, nil) on success.
// Returns (nil, false, nil) when not launched by launchd (ESRCH/ENOENT).
// Returns (nil, false, err) on unexpected errors.
func ActivateSocket(name string) (net.Listener, bool, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var fds *C.int
	var cnt C.size_t

	// launch_activate_socket returns 0 on success, or an errno-style
	// error code directly as its return value (not via errno).
	rc := C.launch_activate_socket(cName, &fds, &cnt)
	if rc != 0 {
		// ESRCH: not launched by launchd
		// ENOENT: socket name not found in plist
		// Both mean "fall back to direct binding"
		if rc == esrch || rc == enoent {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("launch_activate_socket(%q): error code %d", name, rc)
	}
	defer C.free(unsafe.Pointer(fds))

	if cnt == 0 {
		return nil, false, fmt.Errorf("launch_activate_socket(%q): returned 0 fds", name)
	}

	// Use the first file descriptor (plist declares one socket per name)
	fdSlice := unsafe.Slice((*C.int)(fds), int(cnt))
	fd := uintptr(fdSlice[0])

	// Close any extra fds we won't use
	for i := 1; i < int(cnt); i++ {
		_ = os.NewFile(uintptr(fdSlice[i]), "").Close()
	}

	// Wrap fd as net.Listener. net.FileListener dups the fd,
	// so we must close the original os.File to avoid leaking it.
	f := os.NewFile(fd, name)
	listener, err := net.FileListener(f)
	f.Close()
	if err != nil {
		return nil, false, fmt.Errorf("net.FileListener for %q: %w", name, err)
	}

	return listener, true, nil
}
