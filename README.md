# paw-proxy

Stop fighting localhost. Named HTTPS domains for every local dev server.

## Why paw-proxy?

Three Next.js projects. All want port 3000. The second one silently bumps to 3001. Your OAuth callback is hardcoded to `localhost:3000`. Your test fixtures expect it. You've been debugging the wrong app for 20 minutes.

Using git worktrees? Same project, two branches, both on localhost — which tab is which? You just spent 10 minutes testing code that hasn't changed because you're hitting the wrong instance.

**The problem isn't ports. It's identity.** `localhost:3000` doesn't tell you what you're running. A named domain does. And once you have names, HTTPS comes free — real trusted certificates, no browser warnings, OAuth callbacks that just work.

```bash
# Before: port conflicts and confusion
npm run dev  # → localhost:3000... or 3001? which project is this?

# After: named domains, just works
up npm run dev  # → https://myapp.test ✓
```

## Features

- **Zero config** - Just run `up bun dev` and get HTTPS
- **Auto SSL** - Generates trusted certificates on-the-fly
- **WebSocket support** - Hot reload works out of the box
- **Smart naming** - Uses package.json name or directory name
- **Docker Compose** - Auto-discovers services and creates `service.project.test` routes
- **Conflict resolution** - Automatic fallback when a domain is already in use (great for git worktrees)
- **Live dashboard** - Real-time request feed and route status at `https://_paw.test`

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

### Docker Compose

Wrap `docker compose up` to get HTTPS domains for every service with published ports:

```bash
~/projects/myapp$ up docker compose up
Mapping https://frontend.myapp.test -> localhost:3000...
Mapping https://api.myapp.test -> localhost:8080...
2 services live:
   https://frontend.myapp.test
   https://api.myapp.test
------------------------------------------------
```

Services without published ports (like databases) are skipped. The project name comes from your compose config — override it with `-n`:

```bash
# Custom project name
up -n shop docker compose up
# → https://frontend.shop.test, https://api.shop.test

# With compose flags (profiles, custom files)
up docker compose --profile frontend up
up docker compose -f compose.prod.yml up
```

### Dashboard

Visit `https://_paw.test` to see a live dashboard with:
- Active routes and their uptime, request counts, and average latency
- Real-time request feed via Server-Sent Events
- Filter requests by route (click any route row)

### Git Worktrees

Running multiple branches of the same project? paw-proxy handles it automatically. When two instances of `up` register the same name (e.g., from a shared `package.json`), the second instance falls back to its directory name:

```bash
# Main checkout: ~/myapp/
up bun dev
# → https://myapp.test

# Worktree: ~/myapp-feat-auth/
up bun dev
# ⚠️  myapp.test already in use from ~/myapp
#    Using myapp-feat-auth.test instead
# → https://myapp-feat-auth.test
```

You can also set an explicit name with `-n`:

```bash
up -n staging bun dev
# → https://staging.test
```

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
up [-n name] [--restart] <command> [args...]

Options:
  -n name    Custom domain name (default: package.json name or directory)
  --restart  Auto-restart on crash (non-zero exit, single-app mode only)

Docker Compose mode:
  up docker compose up           Auto-discover services, register routes
  up -n shop docker compose up   Override project name portion
  up docker compose --profile frontend up   Compose flags supported

Environment variables set for your command:
  PORT                 - The port your server should listen on (single-app mode)
  APP_DOMAIN           - e.g., myapp.test (single-app mode)
  APP_URL              - e.g., https://myapp.test (single-app mode)
  HTTPS                - "true" (single-app mode)
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

## Inspiration & Prior Art

paw-proxy stands on the shoulders of giants. This project wouldn't exist without:

- **[mkcert](https://github.com/FiloSottile/mkcert)** - The gold standard for local CA generation. We learned a lot from how it handles certificate trust.
- **[puma-dev](https://github.com/puma/puma-dev)** - The original `.test` domain proxy for macOS. Our architecture mirrors many of its ideas.
- **[pow](http://pow.cx/)** - The OG that started it all. RIP.
- **[hotel](https://github.com/typicode/hotel)** - Cross-platform proxy with a nice UI. Inspired our zero-config approach.
- **[caddy](https://caddyserver.com/)** - Automatic HTTPS done right. We borrowed their "just works" philosophy.

We didn't reinvent the wheel — we just modernized it for modern dev workflows where every project needs named HTTPS domains.

## License

MIT
