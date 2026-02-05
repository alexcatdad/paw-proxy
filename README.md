# paw-proxy

Zero-config HTTPS proxy for local macOS development. Get `https://myapp.test` working in seconds.

## Features

- **Zero config** - Just run `up bun dev` and get HTTPS
- **Auto SSL** - Generates trusted certificates on-the-fly
- **WebSocket support** - Hot reload works out of the box
- **Smart naming** - Uses package.json name or directory name
- **Conflict detection** - Warns when domain is already in use

## Installation

```bash
# Download latest release
curl -L https://github.com/alexcatdad/paw-proxy/releases/latest/download/paw-proxy-darwin-universal -o /usr/local/bin/paw-proxy
curl -L https://github.com/alexcatdad/paw-proxy/releases/latest/download/up-darwin-universal -o /usr/local/bin/up
chmod +x /usr/local/bin/paw-proxy /usr/local/bin/up

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

## License

MIT
