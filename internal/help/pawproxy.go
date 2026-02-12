package help

// PawProxyCommand defines the help content for the paw-proxy binary.
var PawProxyCommand = Command{
	Name:    "paw-proxy",
	Summary: "Zero-config HTTPS proxy for local macOS development",
	Usage:   "paw-proxy <command> [options]",
	Subcommands: []Subcommand{
		{
			Name:         "setup",
			Summary:      "Configure DNS, CA, and install daemon (requires sudo)",
			Usage:        "sudo paw-proxy setup",
			RequiresRoot: true,
		},
		{
			Name:         "uninstall",
			Summary:      "Remove all paw-proxy components (requires sudo)",
			Usage:        "sudo paw-proxy uninstall [--brew]",
			RequiresRoot: true,
			Flags: []Flag{
				{Long: "--brew", Desc: "Skip CA removal prompt (used by Homebrew uninstall hook)"},
			},
		},
		{
			Name:    "status",
			Summary: "Show daemon status and registered routes",
		},
		{
			Name:    "run",
			Summary: "Run daemon in foreground (used by launchd)",
		},
		{
			Name:    "logs",
			Summary: "Show daemon logs",
			Usage:   "paw-proxy logs [--tail|-f] [--clear]",
			Flags: []Flag{
				{Short: "-f", Long: "--tail", Desc: "Follow log output in real time"},
				{Long: "--clear", Desc: "Truncate the log file"},
			},
		},
		{
			Name:    "doctor",
			Summary: "Run diagnostics to check system health",
		},
		{
			Name:    "version",
			Summary: "Show version",
		},
	},
	Examples: []Example{
		{Command: "sudo paw-proxy setup", Desc: "Initial setup (creates CA, configures DNS, installs daemon)"},
		{Command: "paw-proxy status", Desc: "Check if daemon is running and see active routes"},
		{Command: "paw-proxy logs -f", Desc: "Follow daemon logs in real time"},
		{Command: "paw-proxy doctor", Desc: "Diagnose common issues"},
	},
	Files: []FilePath{
		{Path: "~/Library/Application Support/paw-proxy/", Desc: "Support directory (CA, socket, config)"},
		{Path: "~/Library/Logs/paw-proxy.log", Desc: "Daemon log file"},
		{Path: "/etc/resolver/test", Desc: "macOS DNS resolver for .test TLD"},
		{Path: "~/Library/LaunchAgents/com.alexcatdad.paw-proxy.plist", Desc: "LaunchAgent for auto-start"},
	},
	SeeAlso: []string{"up(1)"},
}
