// internal/dns/server_test.go
package dns

import (
	"net"
	"strings"
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

func TestValidDNSName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"normal domain", "myapp.test.", true},
		{"single label", "test.", true},
		{"multi-label", "a.b.c.test.", true},
		{"label exactly 63 chars", strings.Repeat("a", 63) + ".test.", true},
		{"label 64 chars exceeds limit", strings.Repeat("a", 64) + ".test.", false},
		{"total exactly 253 chars", buildName(253) + ".", true},
		{"total 254 chars exceeds limit", buildName(254) + ".", false},
		{"empty name with dot only", ".", false},
		{"empty label (double dot)", "a..b.test.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validDNSName(tt.input)
			if got != tt.valid {
				t.Errorf("validDNSName(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

// buildName constructs a domain name of exactly n characters (without trailing dot)
// using labels of up to 63 characters separated by dots.
func buildName(n int) string {
	var parts []string
	remaining := n
	for remaining > 0 {
		labelLen := remaining
		if labelLen > 63 {
			labelLen = 63
		}
		if len(parts) > 0 {
			remaining-- // account for the dot separator
			if remaining <= 0 {
				break
			}
			labelLen = remaining
			if labelLen > 63 {
				labelLen = 63
			}
		}
		parts = append(parts, strings.Repeat("a", labelLen))
		remaining -= labelLen
	}
	return strings.Join(parts, ".")
}

func TestDNSServer_RejectsLongLabel(t *testing.T) {
	// The miekg/dns library rejects labels > 63 octets at the wire-format level,
	// so we cannot send such queries over UDP. Instead, test via validDNSName unit
	// tests (TestValidDNSName/label_64_chars_exceeds_limit) and verify that a
	// label at exactly 63 chars is accepted by the server.
	srv, err := NewServer("127.0.0.1:19355", "test")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Stop()

	go srv.Start()
	time.Sleep(50 * time.Millisecond)

	c := new(dns.Client)
	m := new(dns.Msg)
	// 63-char label is the maximum allowed; should succeed.
	label63 := strings.Repeat("b", 63)
	m.SetQuestion(label63+".test.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:19355")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if r.Rcode != dns.RcodeSuccess {
		t.Errorf("expected success (rcode 0) for 63-char label, got rcode %d", r.Rcode)
	}

	if len(r.Answer) == 0 {
		t.Error("expected answer for valid 63-char label query, got none")
	}
}

func TestDNSServer_AcceptsValidQuery(t *testing.T) {
	srv, err := NewServer("127.0.0.1:19356", "test")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Stop()

	go srv.Start()
	time.Sleep(50 * time.Millisecond)

	c := new(dns.Client)
	m := new(dns.Msg)
	// Label of exactly 63 chars is valid per RFC 1035.
	validLabel := strings.Repeat("a", 63)
	m.SetQuestion(validLabel+".test.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:19356")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if r.Rcode != dns.RcodeSuccess {
		t.Errorf("expected success (rcode 0), got rcode %d", r.Rcode)
	}

	if len(r.Answer) == 0 {
		t.Error("expected answer for valid query, got none")
	}
}

func TestDNSServer_RejectsLongTotalName(t *testing.T) {
	srv, err := NewServer("127.0.0.1:19357", "test")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Stop()

	go srv.Start()
	time.Sleep(50 * time.Millisecond)

	c := new(dns.Client)
	m := new(dns.Msg)
	// Build a name that exceeds 253 characters total (excluding trailing dot)
	// using valid-length labels.
	longName := buildName(254)
	m.SetQuestion(longName+".test.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:19357")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if r.Rcode != dns.RcodeFormatError {
		t.Errorf("expected FORMERR (rcode %d), got rcode %d", dns.RcodeFormatError, r.Rcode)
	}

	if len(r.Answer) != 0 {
		t.Errorf("expected no answers for invalid query, got %d", len(r.Answer))
	}
}
