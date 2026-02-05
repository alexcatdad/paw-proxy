// cmd/paw-proxy/main.go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alexcatdad/paw-proxy/internal/daemon"
)

func main() {
	// Subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			cmdSetup()
			return
		case "uninstall":
			cmdUninstall()
			return
		case "status":
			cmdStatus()
			return
		case "run":
			cmdRun()
			return
		case "version":
			fmt.Println("paw-proxy version 1.0.0")
			return
		}
	}

	// Default: show usage
	fmt.Println("Usage: paw-proxy <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  setup      Configure DNS, CA, and install daemon (requires sudo)")
	fmt.Println("  uninstall  Remove all paw-proxy components")
	fmt.Println("  status     Show daemon status and registered routes")
	fmt.Println("  run        Run daemon in foreground (for launchd)")
	fmt.Println("  version    Show version")
	os.Exit(1)
}

func cmdRun() {
	config := daemon.DefaultConfig()

	// Setup logging
	logFile, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	d, err := daemon.New(config)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	log.Println("paw-proxy daemon starting...")
	if err := d.Run(); err != nil {
		log.Fatalf("Daemon error: %v", err)
	}
}

func cmdSetup() {
	// Will implement in Task 7
	fmt.Println("setup command - to be implemented")
}

func cmdUninstall() {
	// Will implement in Task 10
	fmt.Println("uninstall command - to be implemented")
}

func cmdStatus() {
	// Will implement in Task 9
	fmt.Println("status command - to be implemented")
}
