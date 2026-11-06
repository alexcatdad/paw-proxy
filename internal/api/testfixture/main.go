// Command testfixture serves the daemon's Unix-socket API with a canned
// registry, for the macOS app's DaemonClient tests. It is not shipped.
//
//	go run ./internal/api/testfixture -socket /tmp/x.sock
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/api"
)

func main() {
	socket := flag.String("socket", "", "socket path")
	routes := flag.Int("routes", 2, "number of extra routes beyond the two named fixtures (for large responses)")
	flag.Parse()
	if *socket == "" {
		log.Fatal("-socket is required")
	}

	api.Version, api.Build = "2026.11.1", "7"
	reg := api.NewRouteRegistry(time.Hour)
	must(reg.Register("shop", "http://127.0.0.1:3000", "/Users/dev/src/shop"))
	must(reg.Register("api.shop", "http://127.0.0.1:3001", "/Users/dev/src/shop"))
	for i := 0; i < *routes-2; i++ {
		must(reg.Register("bulk-"+strconv.Itoa(i), "http://127.0.0.1:4000", "/tmp/bulk"))
	}

	srv := api.NewServer(*socket, reg)
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatal(err)
		}
	}()
	log.Printf("ready %s", *socket)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	_ = srv.Stop()
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
