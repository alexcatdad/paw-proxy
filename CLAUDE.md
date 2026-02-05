# CLAUDE.md

## Project Overview

paw-proxy is a zero-config HTTPS proxy for local macOS development. It provides two binaries:
- `paw-proxy` - Daemon and setup/management CLI
- `up` - Command wrapper that registers routes and runs dev servers

## Architecture

```
internal/
├── api/        # Route registry + control API (unix socket)
├── daemon/     # Main daemon orchestrator
├── dns/        # DNS server for .test TLD (port 9353)
├── proxy/      # Reverse proxy with WebSocket support
├── setup/      # macOS setup/uninstall (darwin-only)
└── ssl/        # CA generation + per-domain cert cache

cmd/
├── paw-proxy/  # Main CLI (setup, uninstall, status, run)
└── up/         # Dev server wrapper
```

## Key Patterns

### Build Tags
- `setup_darwin.go` / `uninstall_darwin.go` - macOS-specific code
- `setup_other.go` - Stub for non-macOS platforms (CI compatibility)

### Unix Socket API
The daemon exposes a control API at `~/Library/Application Support/paw-proxy/paw-proxy.sock`:
- `POST /routes` - Register route
- `DELETE /routes/{name}` - Deregister route
- `POST /routes/{name}/heartbeat` - Keep route alive
- `GET /routes` - List routes
- `GET /health` - Health check

### Certificate Flow
1. CA generated once at setup, stored in `~/Library/Application Support/paw-proxy/`
2. Per-domain certs generated on-demand via `CertCache.GetCertificate()`
3. CA trusted in macOS login keychain

## Common Tasks

### Run tests
```bash
go test -v ./...
```

### Build binaries
```bash
go build -o paw-proxy ./cmd/paw-proxy
go build -o up ./cmd/up
```

### Test locally
```bash
sudo ./paw-proxy setup
./integration-tests.sh
```

## CI Notes

- Unit tests run on ubuntu-latest
- Integration tests skipped in CI (require macOS keychain)
- Release builds universal binaries with `lipo`
