// internal/dns/server_test.go
package dns

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNSServer_ResolvesTestDomain(t *testing.T) {
	srv, err := NewServer("127.0.0.1:19353", "test")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Stop()

	go srv.Start()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Query the server
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion("myapp.test.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:19353")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("expected answer, got none")
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", r.Answer[0])
	}

	if !a.A.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %v", a.A)
	}
}

// TestAAAAQuery sends a DNS AAAA query for "example.test" and verifies
// it returns ::1 (IPv6 loopback).
func TestAAAAQuery(t *testing.T) {
	srv, err := NewServer("127.0.0.1:19354", "test")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Stop()

	go srv.Start()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Send AAAA query
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion("example.test.", dns.TypeAAAA)

	r, _, err := c.Exchange(m, "127.0.0.1:19354")
	if err != nil {
		t.Fatalf("DNS AAAA query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("expected AAAA answer, got none")
	}

	aaaa, ok := r.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("expected AAAA record, got %T", r.Answer[0])
	}

	expected := net.ParseIP("::1")
	if !aaaa.AAAA.Equal(expected) {
		t.Errorf("expected ::1, got %v", aaaa.AAAA)
	}
}
