//go:build darwin

package launchd

import (
	"net"
	"syscall"
	"testing"
)

func TestCloseFD_ValidFD(t *testing.T) {
	// Create a socket fd then close it via closeFD. Verify no panic.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("creating test socket: %v", err)
	}
	closeFD(fd)

	// Verify the fd is closed by trying to use it — should fail.
	_, err = syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_TYPE)
	if err == nil {
		t.Error("expected error using closed fd, got nil")
	}
}

func TestValidation_StreamSocketType(t *testing.T) {
	// Create a TCP socket and verify SO_TYPE returns SOCK_STREAM.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("creating test socket: %v", err)
	}
	defer closeFD(fd)

	sockType, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_TYPE)
	if err != nil {
		t.Fatalf("getsockopt SO_TYPE: %v", err)
	}
	if sockType != syscall.SOCK_STREAM {
		t.Errorf("expected SOCK_STREAM (%d), got %d", syscall.SOCK_STREAM, sockType)
	}
}

func TestValidation_ListenOnBoundSocket(t *testing.T) {
	// Create a TCP socket, bind it, listen on it, then verify accept works.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("creating test socket: %v", err)
	}
	defer closeFD(fd)

	// Bind to loopback on any port
	sa := &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}
	if err := syscall.Bind(fd, sa); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// Listen should succeed
	if err := syscall.Listen(fd, syscall.SOMAXCONN); err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Calling listen again should also succeed (idempotent on BSD)
	if err := syscall.Listen(fd, syscall.SOMAXCONN); err != nil {
		t.Fatalf("second listen: %v", err)
	}

	// The socket should have a valid bound address with non-zero port
	boundSA, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatalf("getsockname: %v", err)
	}
	inet4, ok := boundSA.(*syscall.SockaddrInet4)
	if !ok {
		t.Fatalf("expected SockaddrInet4, got %T", boundSA)
	}
	if inet4.Port == 0 {
		t.Error("expected non-zero port after bind")
	}
}

func TestValidation_ListenerAddressCheck(t *testing.T) {
	// Create a proper listening socket via net.Listen, then verify
	// the address check logic from ActivateSocket would pass.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected *net.TCPAddr, got %T", ln.Addr())
	}
	if tcpAddr.Port == 0 {
		t.Error("listener should have a non-zero port")
	}
}
