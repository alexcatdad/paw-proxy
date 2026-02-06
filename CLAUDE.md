# CLAUDE.md

## Project Overview

paw-proxy is a zero-config HTTPS proxy for local macOS development. It provides two binaries:
- `paw-proxy` - Daemon and setup/management CLI
- `up` - Command wrapper that registers routes and runs dev servers

**Go version:** 1.24 (see `go.mod`). Single external dependency: `github.com/miekg/dns v1.1.72`.

## Architecture

```
internal/
├── api/        # Route registry + control API (unix socket)
│   ├── server.go     # HTTP handlers, input validation, socket listener
│   └── routes.go     # RouteRegistry with RWMutex, CRUD + cleanup
├── daemon/     # Main daemon orchestrator
│   └── daemon.go     # Launches 5 goroutines: DNS, API, HTTP, HTTPS, cleanup
├── dns/        # DNS server for .test TLD (port 9353)
│   └── server.go     # UDP-only, responds A/AAAA for *.test → 127.0.0.1/::1
├── proxy/      # Reverse proxy with WebSocket support
│   └── proxy.go      # HTTP forwarding + raw TCP WebSocket proxy
├── setup/      # macOS setup/uninstall (darwin-only)
│   ├── setup_darwin.go    # CA gen, keychain trust, resolver, LaunchAgent
│   ├── setup_other.go     # Stub for non-macOS (returns error)
│   └── uninstall_darwin.go
└── ssl/        # CA generation + per-domain cert cache
    ├── ca.go         # RSA 4096-bit CA, 10-year validity
    └── cert.go       # ECDSA P-256 leaf certs, LRU cache (max 1000)

cmd/
├── paw-proxy/  # Main CLI (setup, uninstall, status, run, version)
│   └── main.go
└── up/         # Dev server wrapper (finds port, registers route, runs command)
    └── main.go
```

## How It Works

1. `sudo paw-proxy setup` — generates CA, trusts it in keychain, creates `/etc/resolver/test`, installs LaunchAgent
2. `paw-proxy run` — starts daemon (DNS on 9353, API on unix socket, HTTP on 80, HTTPS on 443)
3. `up bun dev` — finds free port, registers route with daemon, sets `PORT` env var, runs child command
4. Browser hits `https://myapp.test` → DNS resolves to 127.0.0.1 → HTTPS server generates cert on-demand → proxies to upstream

### Unix Socket API
The daemon exposes a control API at `~/Library/Application Support/paw-proxy/paw-proxy.sock`:
- `POST /routes` — Register route (JSON body: `{name, upstream, dir}`)
- `DELETE /routes/{name}` — Deregister route
- `POST /routes/{name}/heartbeat` — Keep route alive (30s timeout)
- `GET /routes` — List routes (JSON array)
- `GET /health` — Health check (JSON: `{status, version, uptime}`)

## Build, Test, Lint

```bash
# Build
go build -o paw-proxy ./cmd/paw-proxy
go build -o up ./cmd/up

# Unit tests (run these before every PR)
go test -v -race ./...

# Lint (matches CI)
golangci-lint run

# Vet
go vet ./...

# Integration tests (requires macOS with daemon running)
sudo ./paw-proxy setup
./integration-tests.sh

# Coverage per package
go test -cover ./...

# Vulnerability scan (install once: go install golang.org/x/vuln/cmd/govulncheck@latest)
~/go/bin/govulncheck ./...

# Check for latest version of a dependency
go list -m -versions github.com/miekg/dns
```

### Test Coverage (current)
| Package | Coverage | Notes |
|---------|----------|-------|
| api | 61.7% | Good validation tests, needs concurrency tests |
| dns | 82.4% | Missing AAAA record tests |
| proxy | 40.7% | WebSocket proxy has 0% coverage |
| ssl | 65.0% | Missing cache eviction and concurrency tests |
| daemon | 0.0% | No unit tests — needs mocking |
| setup | 0.0% | Platform-specific, tested via integration |
| cmd/* | 0.0% | CLI entry points |

## Coding Conventions

### Patterns to Follow
- **Input validation:** Route names validated with regex `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$` (server.go:21). Upstream validated to localhost-only (SSRF prevention). Directory must be absolute, no traversal.
- **Security comments:** Prefix security-relevant code with `// SECURITY:` comment explaining why
- **Error wrapping:** Use `fmt.Errorf("context: %w", err)` for all error propagation
- **Build tags:** macOS-specific code uses `//go:build darwin`, with `_other.go` stubs
- **Socket permissions:** 0600 for socket, 0600 for private keys, 0700 for support dir
- **No external deps** beyond `miekg/dns` — use stdlib wherever possible

### Patterns to Avoid
- Don't use `log.Fatal` in library code (only in `cmd/` entry points)
- Don't hold locks during I/O or crypto operations
- Don't return pointers to internally-managed structs from locked methods (return copies)
- Don't use `sh -c` for shell commands — use `exec.Command` with explicit args
- Don't ignore errors from `os.Executable()`, `os.UserHomeDir()`, or I/O operations

### PR Workflow
1. Create branch: `fix/issue-{N}` or `feat/issue-{N}`
2. Implement the fix (read the GitHub issue: `gh issue view {N}`)
3. Run `go test -v -race ./...` — all tests must pass
4. Run `go vet ./...` — must be clean
5. If you added new functionality, add tests
6. Commit with descriptive message referencing the issue: `fix: bind HTTP/HTTPS to loopback only (closes #40)`
7. Push and create PR with `gh pr create`
8. Enable auto-merge: `gh pr merge <PR_NUMBER> --auto --squash`

### CodeRabbit Review Handling

After creating a PR, follow this loop to resolve CodeRabbit review threads:

1. **Wait 60 seconds** after PR creation for CodeRabbit to post its review.
2. **Check for unresolved threads:**
   ```bash
   gh pr view <PR_NUMBER> --json reviewThreads --jq '.reviewThreads[] | select(.isResolved == false)'
   ```
3. **For each unresolved thread:**
   - Read the suggestion carefully.
   - If it is a **valid code fix** (not just a style nit), apply the change to the codebase.
   - If it is a **non-actionable comment or style nit**, resolve the thread with a brief explanation.
4. **After applying any code fixes**, commit and push the changes.
5. **Wait 30 seconds** for CodeRabbit to re-review the updated code.
6. **Re-check for unresolved threads** (repeat from step 2).
7. **Repeat until no unresolved threads remain**, up to a maximum of **3 iterations** to avoid infinite loops.

**Resolving a thread via GraphQL:**
```bash
gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "THREAD_ID"}) { thread { isResolved } } }'
```

**Key rules:**
- Do not resolve a thread without reading and considering the suggestion first.
- Do not check for threads earlier than 60 seconds after PR creation — CodeRabbit needs time to analyze.
- Always re-check after force-pushing fixes — new reviews may appear on changed code.
- If 3 iterations pass and threads remain, leave a PR comment summarizing what is unresolved and stop.

## CI Configuration

File: `.github/workflows/ci.yml`

| Job | Runner | What it does |
|-----|--------|-------------|
| lint | ubuntu-latest | `golangci-lint` |
| test | ubuntu-latest | `go test -v -race ./...` |
| build | ubuntu-latest | Cross-compile darwin arm64/amd64 |
| integration | macos-14 | Full setup + daemon + integration-tests.sh |

**CI caveats:**
- CI uses `go-version: '1.22'` but `go.mod` says `1.24` — this is tracked as issue #34
- Integration job runs as root with `sudo` — no login keychain available
- `trustCA()` in setup_darwin.go falls back to System keychain (`/Library/Keychains/System.keychain`)
- LaunchAgent doesn't work under `sudo` in CI — daemon started directly with `./paw-proxy run &`
- Socket permissions fixed with `chmod` after root creates them

## Key Code Locations

### Daemon lifecycle (daemon.go)
- `Run()` at line 95 — launches all goroutines, blocks on HTTPS server
- `cleanupRoutine()` at line 122 — ticker every 10s, currently leaks (no stop)
- `serveHTTP()` at line 129 — HTTP→HTTPS redirect on port 80
- `serveHTTPS()` at line 146 — TLS config, main HTTPS server
- `handleRequest()` at line 179 — route lookup + proxy delegation

### Proxy (proxy.go)
- `ServeHTTP()` at line 48 — HTTP forwarding with X-Forwarded headers
- `handleWebSocket()` at line 95 — hijacks connection, bidirectional io.Copy
- `isWebSocket()` at line 88 — checks Upgrade header

### Route Registry (routes.go)
- `RouteRegistry` struct at line 27 — `map[string]*Route` with `sync.RWMutex`
- `Register()` at line 40 — returns `ConflictError` if name exists
- `Cleanup()` at line 109 — deletes routes past heartbeat timeout
- `LookupByHost()` at line 82 — strips `.test` suffix to find route name

### Certificate Cache (cert.go)
- `GetCertificate()` at line 35 — TLS callback, checks cache then generates
- `generateCert()` at line 95 — ECDSA P-256, 1-year validity, signed by CA
- LRU eviction at line 74 — oldest entry evicted when cache hits 1000

### Input Validation (server.go)
- `validateRouteName()` at line 79 — regex for DNS-safe names
- `validateUpstream()` at line 87 — localhost-only SSRF prevention
- `validateDir()` at line 108 — absolute path, no traversal

### Setup (setup_darwin.go)
- `Run()` at line 24 — 5-step setup: dir, CA, keychain, resolver, LaunchAgent
- `trustCA()` at line 87 — login keychain first, System keychain fallback
- `installLaunchAgent()` at line 155 — plist template with socket declarations

## Backlog

All issues are independently implementable (no blocking dependencies except #13 which needs #3 first).

Use `gh issue view {N}` to read the full issue description before implementing.
Use `gh issue list --state open` to see all open issues.
Use `gh issue list --label P0` (or P1, P2, P3) to filter by priority.
Use `closes #N` in commit message or PR body to auto-close issues on merge.

### Quick Reference

**Tiny fixes (1-4 lines):** #4, #34, #35, #40, #41, #48, #49, #52
**Small fixes (5-20 lines):** #8, #24, #28, #43, #44, #45, #46, #47, #50, #51
**Medium (20-100 lines):** #6, #7, #12, #22, #25, #26, #32, #36, #37, #42, #53
**Large (100+ lines):** #3, #10, #11, #13, #15, #20, #26, #27, #31, #33, #55

### Combined Issues (dependent work merged)
- **#3** = graceful shutdown + ticker leak + socket cleanup
- **#20** = HTTP/2 + WebSocket over HTTP/2
- **#11** = structured logging + TLS errors + access logs
- **#42** = mutable pointer fix + lock contention
- **#55** = version ldflags + Homebrew tap
- **#13** = launchd socket activation + plist fix (blocked by #3)

## Cloud Session Notes (Claude Code Cloud)

Cloud sessions run in a sandboxed Linux container. Key constraints:

### What Works
- Full file system access (read/write/edit)
- Go toolchain (`go build`, `go test -race`, `go vet`)
- Git operations (clone, commit, push) — via local proxy at `127.0.0.1:46667`
- Reading GitHub issues via `WebFetch` on public github.com URLs
- Installing tools with `go install`

### What Doesn't Work
- `gh` CLI — not pre-installed, and GitHub API auth unavailable even if downloaded
- Creating PRs programmatically — no `GH_TOKEN`, proxy doesn't forward API auth
- `golangci-lint` — not pre-installed (skip lint step; `go vet` suffices)
- macOS-specific integration tests — container is Linux
- `sudo` — not available

### Cloud PR Workflow (replaces standard PR Workflow above)
1. Create branch: `fix/issue-{N}` or `feat/issue-{N}`
2. Read the GitHub issue via WebFetch: `https://github.com/alexcatdad/paw-proxy/issues/{N}`
3. Implement the fix
4. Run `go test -v -race ./...` — all tests must pass
5. Run `go vet ./...` — must be clean
6. Commit with descriptive message: `fix: description (closes #{N})`
7. Push branch: `git push -u origin <branch-name>`
8. **Stop here** — output the branch name and a suggested PR title/body. The user will create the PR manually.

## Known Limitations

- HTTP/1.1 only — no HTTP/2 or HTTP/3 (#20)
- HTTP/HTTPS servers bind to all interfaces, not just loopback (#40)
- Proxy overwrites Host header with upstream address (#41)
- SSE connections die after 60s due to WriteTimeout (#24)
- WebSocket has absolute 1-hour deadline, not idle-based (#22)
- `up` doesn't signal process group — child processes orphaned on Ctrl+C (#44)
- Version is hardcoded, not injected at build time (#55)
- `extractConflictDir()` in `cmd/up/main.go` is a no-op (#28)
- `sanitizeName` doesn't lowercase and can produce invalid names (#45)
