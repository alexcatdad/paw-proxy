package help

// UpCommand defines the help content for the up binary.
var UpCommand = Command{
	Name:    "up",
	Summary: "Dev server wrapper — register routes with paw-proxy and run commands",
	Usage:   "up [-n name] [--restart] [--tailnet] <command> [args...]",
	Flags: []Flag{
		{Short: "-n", Arg: "name", Desc: "Custom domain name (default: package.json name or directory)"},
		{Long: "--restart", Desc: "Auto-restart on crash (non-zero exit)"},
		{Long: "--tailnet", Desc: "Also serve on your tailnet with Tailscale Serve (falls back to .test only)"},
	},
	EnvVars: []EnvVar{
		{Name: "PORT", Desc: "Allocated port for your dev server to bind to"},
		{Name: "APP_DOMAIN", Desc: "Domain name, e.g. myapp.test"},
		{Name: "APP_URL", Desc: "Full URL, e.g. https://myapp.test"},
		{Name: "HTTPS", Desc: "Always \"true\""},
		{Name: "APP_TAILNET_URL", Desc: "Tailnet URL, set only when --tailnet is serving"},
		{Name: "NODE_EXTRA_CA_CERTS", Desc: "Path to CA cert (for Node.js HTTPS requests)"},
	},
	Examples: []Example{
		{Command: "up bun dev", Desc: "Run Bun dev server with HTTPS"},
		{Command: "up npm run dev", Desc: "Run npm dev server with HTTPS"},
		{Command: "up -n api bun dev", Desc: "Custom domain: https://api.test"},
		{Command: "up --restart bun dev", Desc: "Auto-restart on crash"},
		{Command: "up --tailnet bun dev", Desc: "Also open it from your phone over Tailscale"},
	},
	SeeAlso: []string{"paw-proxy(1)"},
}
