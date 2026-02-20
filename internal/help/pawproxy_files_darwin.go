//go:build darwin

package help

func init() {
	PawProxyCommand.Files = []FilePath{
		{Path: "~/Library/Application Support/paw-proxy/", Desc: "Support directory (CA, socket)"},
		{Path: "~/Library/Logs/paw-proxy.log", Desc: "Daemon log file"},
		{Path: "/etc/resolver/test", Desc: "macOS DNS resolver for .test TLD"},
		{Path: "~/Library/LaunchAgents/dev.paw-proxy.plist", Desc: "LaunchAgent for auto-start"},
	}
}
