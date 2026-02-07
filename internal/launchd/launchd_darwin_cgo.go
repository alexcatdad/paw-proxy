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
	"syscall"
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
	fd := int(fdSlice[0])

	// Close any extra fds we won't use
	for i := 1; i < int(cnt); i++ {
		_ = os.NewFile(uintptr(fdSlice[i]), "").Close()
	}

	// Validate fd is a TCP stream socket. Without this check, a misconfigured
	// plist could pass a UDP or Unix socket, causing confusing accept() errors.
	sockType, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_TYPE)
	if err != nil {
		closeFD(fd)
		return nil, false, fmt.Errorf("getsockopt SO_TYPE for %q fd %d: %w", name, fd, err)
	}
	if sockType != syscall.SOCK_STREAM {
		closeFD(fd)
		return nil, false, fmt.Errorf("launchd socket %q: expected SOCK_STREAM (%d), got %d", name, syscall.SOCK_STREAM, sockType)
	}

	// Ensure the socket is in listening state. Launchd calls listen() when
	// SockPassive=true in the plist, but we call it defensively in case the
	// plist was misconfigured. On BSD, listen() on an already-listening socket
	// just updates the backlog — safe to call unconditionally.
	if err := syscall.Listen(fd, syscall.SOMAXCONN); err != nil {
		closeFD(fd)
		return nil, false, fmt.Errorf("listen() on launchd socket %q fd %d: %w", name, fd, err)
	}

	// Wrap fd as net.Listener. net.FileListener dups the fd,
	// so we must close the original os.File to avoid leaking it.
	f := os.NewFile(uintptr(fd), name)
	listener, err := net.FileListener(f)
	f.Close()
	if err != nil {
		return nil, false, fmt.Errorf("net.FileListener for %q: %w", name, err)
	}

	// Verify the listener has a valid bound address. A port of 0 indicates
	// the socket wasn't properly bound by launchd (the 0.0.0.0:0 symptom).
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok && tcpAddr.Port == 0 {
		listener.Close()
		return nil, false, fmt.Errorf("launchd socket %q has invalid address %s", name, listener.Addr())
	}

	return listener, true, nil
}

// closeFD closes a raw file descriptor by wrapping it in os.File.
func closeFD(fd int) {
	os.NewFile(uintptr(fd), "").Close()
}
