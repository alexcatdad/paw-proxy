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
3. CA trusted in macOS login keychain (falls back to System keychain for CI/headless)

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
- Integration tests run on macos-14 (Apple Silicon runner)
- CI creates support dir and socket as root — permissions are fixed via `chmod` steps
- LaunchAgent doesn't work under sudo in CI — daemon is started directly with `./paw-proxy run &`
- Login keychain unavailable for root in CI — `trustCA()` falls back to System keychain (`/Library/Keychains/System.keychain`)
- Integration test script uses `curl -sf` (not `grep -q ""`) to check empty-body responses
- Release builds universal binaries with `lipo`

## Backlog & Issue Tracking

All improvements, bugs, and feature requests are tracked as GitHub Issues with labels:

### Priority Labels
- `P0` - Critical: goroutine leaks, missing shutdown, data correctness
- `P1` - High: silent failures, broken features (SSE, dead code), protocol support
- `P2` - Medium: DX improvements, operational robustness, test coverage
- `P3` - Low/future: nice-to-haves, optimizations, edge cases

### Category Labels
- `improvement` - Enhancement or optimization to existing code
- `bug` - Something that's broken or not working correctly
- `feature` - New functionality
- `dx` - Developer experience improvement

### Key Issue Groups

**Daemon lifecycle (P0):** #3 graceful shutdown, #4 WebSocket goroutine leak, #5 ticker leak

**Error handling (P1):** #6 silent goroutine failures, #8 ignored errors, #24 SSE killed by timeout

**Protocol support (P1):** #20 HTTP/2, #21 WebSocket over HTTP/2 (blocks #20)

**Concurrency (P1-P2):** #7 CertCache race condition, #18 route cleanup lock contention

**DX commands (P2-P3):** #26 `doctor` command, #27 `logs` command, #31 friendly error pages

**Testing (P2):** #10 daemon + WebSocket unit tests

**Dead code / bugs:** #28 `extractConflictDir` no-op (P1), #32 `up` socket-file-only check (P2)

Run `gh issue list --label P0` (or P1, P2, P3) to filter by priority.

## Known Limitations

- HTTP/1.1 only — no HTTP/2 or HTTP/3 (see #20)
- SSE connections die after 60s due to WriteTimeout (see #24)
- WebSocket has absolute 1-hour deadline, not idle-based (see #22)
- No request access logging (see #30)
- Version is hardcoded, not injected at build time (see #29)
- `extractConflictDir()` in `cmd/up/main.go` is a no-op — conflict fallback doesn't work (see #28)
