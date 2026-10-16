package main

import (
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/miekg/dns"
)

func TestDoctorDNSAddresses(t *testing.T) {
	for _, mode := range []string{"IPv4 only", "both families", "no answers", "server failure"} {
		t.Run(mode, func(t *testing.T) {
			pc, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ready := make(chan struct{})
			srv := &dns.Server{PacketConn: pc, NotifyStartedFunc: func() { close(ready) }, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
				reply := new(dns.Msg)
				reply.SetReply(req)
				if mode == "server failure" {
					reply.Rcode = dns.RcodeServerFailure
				} else if mode != "no answers" {
					header := dns.RR_Header{Name: req.Question[0].Name, Rrtype: req.Question[0].Qtype, Class: dns.ClassINET}
					if header.Rrtype == dns.TypeA {
						reply.Answer = []dns.RR{&dns.A{Hdr: header, A: net.ParseIP("127.0.0.1")}}
					} else if mode == "both families" {
						reply.Answer = []dns.RR{&dns.AAAA{Hdr: header, AAAA: net.ParseIP("::1")}}
					}
				}
				if err := w.WriteMsg(reply); err != nil {
					t.Error(err)
				}
			})}
			done := make(chan error, 1)
			go func() { done <- srv.ActivateAndServe() }()
			<-ready
			t.Cleanup(func() {
				if err := srv.Shutdown(); err != nil {
					t.Error(err)
				}
				if err := <-done; err != nil {
					t.Error(err)
				}
			})
			ips, err := doctorDNSAddresses(pc.LocalAddr().String(), "doctor.test")
			wantErr := mode == "no answers" || mode == "server failure"
			if (err != nil) != wantErr {
				t.Fatalf("addresses=%v err=%v, want error=%t", ips, err, wantErr)
			}
			if wantErr {
				return
			}
			want := "[127.0.0.1]"
			if mode == "both families" {
				want = "[127.0.0.1 ::1]"
			}
			if got := fmt.Sprint(ips); got != want {
				t.Fatalf("addresses=%s, want %s", got, want)
			}
		})
	}
}

func TestDoctorTLS(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(address, func(t *testing.T) {
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Backend availability must not affect TLS diagnostics.
				w.WriteHeader(http.StatusBadGateway)
			}))
			srv.Listener.Close()
			srv.Listener = listener
			srv.StartTLS()
			defer srv.Close()
			roots := x509.NewCertPool()
			roots.AddCert(srv.Certificate())
			destination := listener.Addr().String()
			if err := doctorTLS("example.com", destination, roots); err != nil {
				t.Fatalf("trusted TLS with unavailable backend: %v", err)
			}
			if err := doctorTLS("doctor.test", destination, roots); err == nil {
				t.Fatal("accepted incorrect hostname")
			}
			if err := doctorTLS("example.com", destination, x509.NewCertPool()); err == nil {
				t.Fatal("accepted incorrect CA")
			}
			if err := doctorTLS("example.com", destination, nil); err == nil {
				t.Fatal("accepted missing CA")
			}
		})
	}

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if err := doctorTLS("doctor.test", srv.Listener.Addr().String(), x509.NewCertPool()); err == nil {
		t.Fatal("TCP/HTTP listener without TLS reported healthy")
	}
	if err := doctorTLS("doctor.test", "192.0.2.1:443", x509.NewCertPool()); err == nil {
		t.Fatal("non-loopback address accepted")
	}
}
