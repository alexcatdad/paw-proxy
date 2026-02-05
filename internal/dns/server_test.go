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
