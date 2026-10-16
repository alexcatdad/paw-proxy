package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

// Query both families: a UDP dial alone succeeds even if no DNS server exists.
func doctorDNSAddresses(server, host string) ([]net.IP, error) {
	client := &dns.Client{Timeout: 2 * time.Second}
	var addresses []net.IP
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(host), qtype)
		reply, _, err := client.Exchange(msg, server)
		if err != nil {
			return nil, fmt.Errorf("%s query: %w", dns.TypeToString[qtype], err)
		}
		if reply.Rcode != dns.RcodeSuccess || reply.Truncated {
			return nil, fmt.Errorf("%s query: invalid DNS response (rcode %s, truncated %t)", dns.TypeToString[qtype], dns.RcodeToString[reply.Rcode], reply.Truncated)
		}
		for _, rr := range reply.Answer {
			switch rr := rr.(type) {
			case *dns.A:
				addresses = append(addresses, rr.A)
			case *dns.AAAA:
				addresses = append(addresses, rr.AAAA)
			}
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no A or AAAA answers for %s", host)
	}
	return addresses, nil
}

// Pin the destination while retaining SNI and hostname/CA verification. A TCP
// connection alone could hit another service; HTTP status depends on the backend.
func doctorTLS(host, address string, roots *x509.CertPool) error {
	ip, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(ip).IsLoopback() {
		return fmt.Errorf("expected a loopback destination, got %s", address)
	}
	if roots == nil {
		return fmt.Errorf("paw-proxy CA unavailable")
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", address, &tls.Config{
		ServerName: host,
		RootCAs:    roots,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return err
	}
	return conn.Close()
}
