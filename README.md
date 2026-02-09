# paw-proxy

Zero-config HTTPS proxy for local macOS development. Get `https://myapp.test` working in seconds.

## Why paw-proxy?

Local development with HTTPS is painful. You need it for:
- **OAuth callbacks** that require HTTPS redirect URIs
- **Secure cookies** with `SameSite=None` or `Secure` flags
- **Service workers** that only work on secure origins
- **Mixed content** issues when your API is HTTPS but dev server is HTTP
- **Production parity** - test how your app actually behaves

Existing solutions are frustrating:
- **mkcert** - Great for certs, but you still need nginx/caddy config per project
- **ngrok/Cloudflare Tunnel** - External dependency, latency, rate limits
- **Self-signed certs** - Browser warnings, manual trust, breaks fetch/curl

paw-proxy gives you `https://myapp.test` with zero config. Just prefix your dev command with `up`:

```bash
# Before: http://localhost:3000 with HTTPS headaches
npm run dev

# After: https://myapp.test just works
up npm run dev
```

## Features

- **Zero config** - Just run `up bun dev` and get HTTPS
- **Auto SSL** - Generates trusted certificates on-the-fly
- **WebSocket support** - Hot reload works out of the box
- **Smart naming** - Uses package.json name or directory name
- **Conflict detection** - Warns when domain is already in use

## Installation

```bash
brew install alexcatdad/tap/paw-proxy

# Run setup (creates CA, configures DNS, installs daemon)
sudo paw-proxy setup
```

## Usage

```bash
# Wrap any dev server command
up bun dev
up npm run dev
up yarn dev

# Custom domain name
up -n myapp npm start

# Check status
paw-proxy status
```

Your app is now available at `https://<name>.test`

## How It Works

1. **DNS** - A local DNS server resolves `*.test` to `127.0.0.1`
2. **SSL** - A trusted CA generates certificates for each domain on-the-fly
3. **Proxy** - HTTPS requests are proxied to your dev server's local port
4. **Auto-port** - `up` finds a free port and sets `PORT` environment variable

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Browser   │────▶│  paw-proxy   │────▶│  Dev Server │
│             │     │  (port 443)  │     │  (dynamic)  │
└─────────────┘     └──────────────┘     └─────────────┘
                           │
                    ┌──────┴──────┐
                    │  DNS Server │
                    │  (port 9353)│
                    └─────────────┘
```

## Commands

### paw-proxy

| Command | Description |
|---------|-------------|
| `setup` | Configure DNS, CA, and install daemon (requires sudo) |
| `uninstall` | Remove all paw-proxy components |
| `status` | Show daemon status and registered routes |
| `run` | Run daemon in foreground (for launchd) |
| `version` | Show version |

### up

```
up [-n name] <command> [args...]

Options:
  -n name    Custom domain name (default: package.json name or directory)

Environment variables set for your command:
  PORT                 - The port your server should listen on
  APP_DOMAIN           - e.g., myapp.test
  APP_URL              - e.g., https://myapp.test
  HTTPS                - "true"
  NODE_EXTRA_CA_CERTS  - Path to CA cert (for Node.js HTTPS requests)
```

## Troubleshooting

### Firefox doesn't trust the certificate

Firefox uses its own certificate store. Install NSS:

```bash
brew install nss
paw-proxy setup  # Re-run to update Firefox
```

### "Daemon not running" error

```bash
# Check status
paw-proxy status

# Re-run setup
sudo paw-proxy setup
```

### Port 80/443 already in use

Stop any other web servers (nginx, Apache, etc.) before running setup.

## Uninstall

```bash
paw-proxy uninstall
```

## Security

paw-proxy is designed with a defense-in-depth approach for local development:

- **Loopback-only binding** — HTTP, HTTPS, and DNS servers all bind to `127.0.0.1`, never to external interfaces
- **SSRF prevention** — Upstream targets are validated as localhost/loopback at both the API layer and the transport layer (double-gated)
- **No shell injection** — All subprocess calls use explicit argument lists via `exec.Command`, never `sh -c`
- **Secure socket permissions** — The Unix control socket is created with `0600` via umask, avoiding TOCTOU races between creation and chmod
- **Input validation** — Route names are regex-checked, upstreams are restricted to loopback, and directory paths must be absolute with no traversal sequences
- **Rate limiting** — All API endpoints are rate-limited to prevent abuse from runaway scripts
- **Resource caps** — Route registry is capped at 100 entries; certificate cache is capped at 1000 with LRU eviction
- **TLS hardening** — Minimum TLS 1.2 with a curated set of strong cipher suites (ECDHE + AES-GCM / ChaCha20)
- **Constrained CA** — The generated CA uses RSA 4096-bit keys with `MaxPathLen=0`, preventing it from signing intermediate CAs
- **Private key protection** — CA private key is written with `0600` (owner-only) permissions
- **SNI required** — Connections without a Server Name Indication are rejected, preventing cert misissuance for IP-based requests
- **XSS prevention** — All dynamic content in error pages is HTML-escaped
- **Launchd socket validation** — Socket activation validates the file descriptor is a TCP stream socket with a valid bound address before use
- **Request body limits** — API request bodies are capped at 1MB
- **Minimal dependencies** — Only one external dependency (`miekg/dns`), keeping the supply chain attack surface small
- **Clean uninstall** — `paw-proxy uninstall` removes the CA from your keychain, the DNS resolver, and the LaunchAgent

## Development

```bash
# Build
go build -o paw-proxy ./cmd/paw-proxy
go build -o up ./cmd/up

# Test
go test -v ./...

# Integration tests (requires setup)
sudo ./paw-proxy setup
./integration-tests.sh
```

## Inspiration & Prior Art

paw-proxy stands on the shoulders of giants. This project wouldn't exist without:

- **[mkcert](https://github.com/FiloSottile/mkcert)** - The gold standard for local CA generation. We learned a lot from how it handles certificate trust.
- **[puma-dev](https://github.com/puma/puma-dev)** - The original `.test` domain proxy for macOS. Our architecture mirrors many of its ideas.
- **[pow](http://pow.cx/)** - The OG that started it all. RIP.
- **[hotel](https://github.com/typicode/hotel)** - Cross-platform proxy with a nice UI. Inspired our zero-config approach.
- **[caddy](https://caddyserver.com/)** - Automatic HTTPS done right. We borrowed their "just works" philosophy.

We didn't reinvent the wheel - we just modernized it for 2024+ dev workflows where every project needs HTTPS yesterday.

## License

MIT
