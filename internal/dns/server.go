// internal/dns/server.go
package dns

import (
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
)

type Server struct {
	addr   string
	tld    string
	server *dns.Server
}

func NewServer(addr, tld string) (*Server, error) {
	s := &Server{
		addr: addr,
		tld:  tld,
	}

	s.server = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: dns.HandlerFunc(s.handleRequest),
	}

	return s, nil
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	return s.server.Shutdown()
}

// maxLabelLen is the maximum length of a single DNS label per RFC 1035 section 2.3.4.
const maxLabelLen = 63

// maxNameLen is the maximum length of a full domain name (without trailing dot) per RFC 1035.
const maxNameLen = 253

// validDNSName checks that a fully-qualified DNS name conforms to RFC 1035
// length limits: each label ≤ 63 octets and total name ≤ 253 characters
// (excluding the trailing dot).
func validDNSName(name string) bool {
	// Strip the trailing dot for length checking (FQDN form)
	n := strings.TrimSuffix(name, ".")
	if len(n) > maxNameLen || len(n) == 0 {
		return false
	}
	for _, label := range strings.Split(n, ".") {
		if len(label) > maxLabelLen || len(label) == 0 {
			return false
		}
	}
	return true
}

func (s *Server) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		name := strings.ToLower(q.Name)

		// Validate name length per RFC 1035 before processing.
		if !validDNSName(name) {
			m.Rcode = dns.RcodeFormatError
			break
		}

		if !strings.HasSuffix(name, "."+s.tld+".") {
			continue
		}

		switch q.Qtype {
		case dns.TypeA:
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP("127.0.0.1"),
			}
			m.Answer = append(m.Answer, rr)

		case dns.TypeAAAA:
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				AAAA: net.ParseIP("::1"),
			}
			m.Answer = append(m.Answer, rr)
		}
	}

	if err := w.WriteMsg(m); err != nil {
		log.Printf("dns: write response error: %v", err)
	}
}
