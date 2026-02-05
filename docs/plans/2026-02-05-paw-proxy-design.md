# paw-proxy Design Document

**Version:** 1.0
**Date:** 2026-02-05
**Status:** Ready for implementation

## Overview

paw-proxy is a zero-config HTTPS proxy for local development on macOS. Spiritual successor to [puma-dev](https://github.com/puma/puma-dev), but language-agnostic with an explicit `up` command model.

### Goals

- Single `sudo paw-proxy setup` for one-time installation
- `up <command>` wraps any dev server with automatic HTTPS on `.test` domain
- No configuration files, no symlinks, no magic
- Support macOS 10.15 (Catalina) through 15 (Sequoia)

### Non-Goals (v1)

- Linux support
- Multiple TLD support
- Configuration files
- Process management (auto-restart apps)

---

## Commands

### `paw-proxy setup` (requires sudo)

One-time setup that:
1. Creates `/etc/resolver/test` pointing to localhost:9353
2. Generates CA certificate in `~/Library/Application Support/paw-proxy/`
3. Adds CA to login keychain via `security add-trusted-cert`
4. Installs LaunchAgent for daemon (ports 80/443 via socket activation)

Prints note about macOS "Background Items Added" notification (expected on Ventura+).

### `paw-proxy uninstall`

Removes:
- LaunchAgent plist
- `/etc/resolver/test`
- Support directory

Prompts: "Also remove CA certificate from keychain? [y/n]"

With `--brew` flag: Full cleanup without prompts (for Homebrew post_uninstall).

### `paw-proxy status`

Shows:
- Daemon running/stopped
- Uptime
- Registered routes (name, upstream, registered time)
- CA certificate expiry date
- Recent request count

### `up [-n name] <command...>`

Wraps a dev server with HTTPS proxy:

1. Check daemon running (via socket), error if not: "Run: sudo paw-proxy setup"
2. Find free port (bind to `:0`, get assigned port, close)
3. Determine app name:
   - `-n` flag (explicit, errors on conflict)
   - `package.json` name (falls back to directory on conflict)
   - Directory basename (errors on conflict)
4. Register route with daemon
5. Set environment variables and exec command
6. Send heartbeat every 10s
7. On exit: deregister route

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  paw-proxy daemon (via LaunchAgent)                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ DNS Server  │  │ HTTP Proxy  │  │ Control API         │  │
│  │ port 9353   │  │ ports 80/443│  │ unix socket         │  │
│  │             │  │             │  │                     │  │
│  │ responds to │  │ routes reqs │  │ register/deregister │  │
│  │ *.test      │  │ by hostname │  │ routes, heartbeat   │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  up binary                                                  │
├─────────────────────────────────────────────────────────────┤
│  • Finds free port                                          │
│  • Calls control API to register route                      │
│  • Sends heartbeat every 10s                                │
│  • Exec's the user command with PORT env                    │
│  • Cleanup on exit                                          │
└─────────────────────────────────────────────────────────────┘
```

---

## Files

| Path | Purpose |
|------|---------|
| `/etc/resolver/test` | macOS DNS override for *.test |
| `~/Library/Application Support/paw-proxy/ca.crt` | Root CA certificate |
| `~/Library/Application Support/paw-proxy/ca.key` | Root CA private key |
| `~/Library/Application Support/paw-proxy/paw-proxy.sock` | Control API socket |
| `~/Library/LaunchAgents/dev.paw-proxy.plist` | LaunchAgent config |
| `~/Library/Logs/paw-proxy.log` | Daemon logs |

---

## Control API

Unix socket at `~/Library/Application Support/paw-proxy/paw-proxy.sock`

### Endpoints

```
POST /routes
{"name": "myapp", "upstream": "localhost:54321"}
→ 200 OK

DELETE /routes/{name}
→ 200 OK

POST /routes/{name}/heartbeat
→ 200 OK

GET /routes
→ [{"name": "myapp", "upstream": "localhost:54321", "registered": "...", "lastHeartbeat": "..."}]

GET /health
→ {"status": "ok", "version": "1.0.0", "uptime": "2h30m"}
```

---

## SSL/TLS

### CA Generation (on setup)

- RSA 2048-bit root CA
- Valid for 10 years
- Subject: "paw-proxy CA"
- Stored in `~/Library/Application Support/paw-proxy/`
- Added to login keychain via `security add-trusted-cert`

### Per-Domain Certificates (on-demand)

- ECDSA P-256
- Valid for 1 year
- Generated when first request hits domain
- Cached in memory (LRU, 1000 entries)
- Regenerated on daemon restart

---

## Proxy Behavior

### Request Flow

1. TLS handshake → generate/cache cert for SNI hostname
2. Route lookup → find upstream by hostname
3. Reverse proxy → forward to `http://localhost:{port}`
4. Response → stream back to client

### Headers Injected

- `X-Forwarded-For: {client_ip}`
- `X-Forwarded-Proto: https`
- `X-Forwarded-Host: {hostname}`

### WebSocket Support

- Detect `Upgrade: websocket` header
- Hijack connection and proxy bidirectionally
- Required for HMR (Vite, Next.js, etc.)

### IPv4/IPv6 Handling

When connecting to upstream, try both:
1. `127.0.0.1:{port}` (IPv4)
2. `[::1]:{port}` (IPv6)

Use first successful connection (Happy Eyeballs approach).

### No Route Found

Return 502 with helpful message:
```
No app registered for myapp.test

Run: up -n myapp <your-dev-command>
```

---

## Environment Variables

`up` sets these before exec'ing the command:

| Variable | Example | Purpose |
|----------|---------|---------|
| `PORT` | `54321` | Port the app should listen on |
| `APP_DOMAIN` | `myapp.test` | The assigned domain |
| `APP_URL` | `https://myapp.test` | Full URL |
| `HTTPS` | `true` | Indicates HTTPS is active |
| `NODE_EXTRA_CA_CERTS` | `~/Library/Application Support/paw-proxy/ca.crt` | For Node.js CA trust |

---

## Route Lifecycle

### Registration

1. `up` finds free port
2. `up` POSTs to `/routes` with name + upstream
3. If name conflict from same directory: error
4. If name conflict from different directory: fallback to directory name
5. Daemon adds route, starts accepting traffic

### Heartbeat

- `up` sends POST to `/routes/{name}/heartbeat` every 10s
- Daemon removes route if no heartbeat for 30s
- Handles ungraceful exits (kill -9, crash, power loss)

### Deregistration

- On normal exit: `up` sends DELETE to `/routes/{name}`
- On timeout: daemon auto-removes stale route

---

## macOS Compatibility

| Version | Codename | Notes |
|---------|----------|-------|
| 10.15 | Catalina | Minimum supported |
| 11 | Big Sur | `add-trusted-cert` requires user interaction |
| 12 | Monterey | |
| 13 | Ventura | "Background Items Added" notification |
| 14 | Sonoma | |
| 15 | Sequoia | Test thoroughly (puma-dev has issues) |

### Privileged Ports

Use launchd socket activation:
- launchd binds 80/443
- Passes file descriptors to daemon
- Daemon runs as user, not root

---

## Edge Cases

### Firefox

Doesn't use system keychain. On setup, warn:
```
Note: Firefox requires 'nss' for certificate trust.
Install with: brew install nss
Then re-run: paw-proxy setup
```

### Node.js

Doesn't use system CA store. `up` automatically sets `NODE_EXTRA_CA_CERTS`.

### Worktrees

Same `package.json` name, different directories:
- First instance gets `myapp.test`
- Second instance falls back to directory name: `myapp-feature.test`
- Prints message explaining the fallback

### Signal Handling

`up` forwards signals to child:
1. Trap SIGINT, SIGTERM, SIGHUP
2. Forward to child process
3. Wait for child (with timeout)
4. Deregister route
5. Exit with child's exit code

---

## Distribution

### Homebrew Tap

```ruby
class PawProxy < Formula
  desc "Zero-config HTTPS for local development"
  homepage "https://github.com/..."

  # Universal binary (arm64 + amd64)
  url "..."

  def install
    bin.install "paw-proxy"
    bin.install "up"
  end

  def post_uninstall
    system "paw-proxy", "uninstall", "--brew"
  end

  def caveats
    <<~EOS
      To complete setup, run:
        sudo paw-proxy setup
    EOS
  end
end
```

### Direct Download

GitHub releases with:
- `paw-proxy-darwin-arm64` (Apple Silicon)
- `paw-proxy-darwin-amd64` (Intel)
- `paw-proxy-darwin-universal` (both)
- `up-darwin-arm64`
- `up-darwin-amd64`
- `up-darwin-universal`

### Build

```bash
# No CGO required
GOOS=darwin GOARCH=arm64 go build -o paw-proxy-arm64 ./cmd/paw-proxy
GOOS=darwin GOARCH=amd64 go build -o paw-proxy-amd64 ./cmd/paw-proxy
lipo -create -output paw-proxy paw-proxy-arm64 paw-proxy-amd64

GOOS=darwin GOARCH=arm64 go build -o up-arm64 ./cmd/up
GOOS=darwin GOARCH=amd64 go build -o up-amd64 ./cmd/up
lipo -create -output up up-arm64 up-amd64
```

---

## CI/CD Pipeline

### GitHub Actions

**On every push/PR:**
- Lint (golangci-lint)
- Unit tests
- Build (verify compilation)

**On PRs to main + release tags:**
- All of the above
- Integration tests on macOS runner (actually run setup, proxy, routes)

**On version tags (v*):**
- All of the above
- Build universal binaries (arm64 + amd64)
- Create GitHub Release with artifacts
- Update Homebrew tap formula

### Workflows

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: golangci/golangci-lint-action@v4

  unit-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test -v -race ./...

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: |
          GOOS=darwin GOARCH=arm64 go build ./cmd/paw-proxy
          GOOS=darwin GOARCH=amd64 go build ./cmd/paw-proxy
```

```yaml
# .github/workflows/integration.yml
name: Integration Tests
on:
  pull_request:
    branches: [main]
  push:
    tags: ['v*']

jobs:
  integration:
    runs-on: macos-14  # Apple Silicon runner
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go build -o paw-proxy ./cmd/paw-proxy
      - run: go build -o up ./cmd/up
      - run: sudo ./paw-proxy setup
      - run: ./integration-tests.sh
```

```yaml
# .github/workflows/release.yml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: macos-14
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5

      - name: Build binaries
        run: |
          # paw-proxy
          GOOS=darwin GOARCH=arm64 go build -o dist/paw-proxy-arm64 ./cmd/paw-proxy
          GOOS=darwin GOARCH=amd64 go build -o dist/paw-proxy-amd64 ./cmd/paw-proxy
          lipo -create -output dist/paw-proxy dist/paw-proxy-arm64 dist/paw-proxy-amd64

          # up
          GOOS=darwin GOARCH=arm64 go build -o dist/up-arm64 ./cmd/up
          GOOS=darwin GOARCH=amd64 go build -o dist/up-amd64 ./cmd/up
          lipo -create -output dist/up dist/up-arm64 dist/up-amd64

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            dist/paw-proxy
            dist/paw-proxy-arm64
            dist/paw-proxy-amd64
            dist/up
            dist/up-arm64
            dist/up-amd64

      - name: Update Homebrew Tap
        run: |
          # Clone tap repo, update formula, push
          # (details depend on tap repo setup)
```

### Test Categories

**Unit tests** (fast, no system changes):
- DNS response generation
- Certificate generation
- Route matching logic
- Config parsing
- JSON API serialization

**Integration tests** (requires macOS, may need sudo):
- Full setup flow
- Route registration/deregistration
- Proxy request flow
- WebSocket upgrade
- Heartbeat timeout
- Signal handling

---

## Future Considerations (v2+)

- Linux support
- Multiple TLDs (`.test`, `.localhost`)
- Config file for power users
- `paw-proxy logs` command (tail daemon logs)
- Automatic `brew install nss` option
- Browser extension for status indicator
