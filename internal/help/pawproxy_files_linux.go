//go:build linux

package help

func init() {
	PawProxyCommand.Files = []FilePath{
		{Path: "~/.local/share/paw-proxy/", Desc: "Support directory (CA, socket)"},
		{Path: "~/.local/state/paw-proxy/paw-proxy.log", Desc: "Daemon log file"},
		{Path: "/etc/systemd/resolved.conf.d/paw-proxy.conf", Desc: "systemd-resolved DNS stub zone"},
		{Path: "~/.config/systemd/user/paw-proxy.service", Desc: "Systemd user unit for auto-start"},
	}
}
