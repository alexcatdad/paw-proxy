# Docker Compose Support for `up`

**Date:** 2026-02-19
**Status:** Approved

## Problem

paw-proxy's `up` command wraps single dev server commands (`up bun dev`, `up npm start`), but many teams run local services via Docker Compose. There's no way to get `.test` HTTPS domains for Docker containers without manual route registration.

## Decision

**Host proxy, containerized apps**: paw-proxy stays on macOS host. `up` wraps `docker compose up`, auto-discovers services from compose config, and registers multi-level routes (`service.project.test`).

**Service discovery via `docker compose config --format json`**: Docker CLI normalizes the compose file (handles profiles, env interpolation, extends, multiple files) and outputs JSON. No YAML parsing dependency needed.

## Architecture

```
up docker compose --profile frontend up -d
│
├─ Detect: "docker compose" + "up" → Docker Compose mode
├─ Capture compose flags: ["--profile", "frontend"]
│
├─ Run: docker compose --profile frontend config --format json
│  └─ Parse JSON → project name + services with published ports
│
├─ Override project name if -n flag set
├─ Sanitize names through sanitizeName()
│
├─ Register routes:
│  ├─ frontend.myapp → localhost:3000
│  └─ api.myapp → localhost:8080
│
├─ Start: docker compose --profile frontend up -d
│  (full original args, as child process with process group)
│
├─ Heartbeat loop: every 10s for all registered routes
│
└─ On exit: deregister all routes, forward signal to process group
```

## Route Naming

Format: `{service}.{project}.test`

- Project name resolved by Docker Compose (precedence: `--project-name` > `COMPOSE_PROJECT_NAME` env > compose file `name:` > directory)
- `up -n` flag overrides the project portion (routing concern, separate from Docker's `--project-name`)
- Only services with published ports are registered
- Names sanitized through existing `sanitizeName()` for DNS safety

Examples:
```
~/projects/myapp$ up docker compose up
  → frontend.myapp.test, api.myapp.test

~/projects/myapp$ up -n shop docker compose up
  → frontend.shop.test, api.shop.test

~/projects/myapp$ up docker compose --profile frontend up
  → frontend.myapp.test (only profiled services)
```

## Changes Required

### internal/api/server.go (1 line)

Route name regex: `^[a-zA-Z][a-zA-Z0-9_-]{0,62}$` → `^[a-zA-Z][a-zA-Z0-9._-]{0,62}$`

Allow dots for subdomain-style route names.

### internal/api/routes.go (~5 lines)

`ExtractName()` rewrite — strip `.test` suffix instead of splitting on first dot:

```go
func ExtractName(host string) string {
    if h, _, err := net.SplitHostPort(host); err == nil {
        host = h
    }
    return strings.TrimSuffix(host, ".test")
}
```

### internal/api/routes_test.go (~20 lines)

Tests for dotted route names and new `ExtractName` behavior:
- `frontend.myapp.test` → `frontend.myapp`
- `frontend.myapp.test:443` → `frontend.myapp`
- `myapp.test` → `myapp` (backwards compatible)

### cmd/up/main.go (~150 lines)

Docker Compose mode:
- Detection: scan for `up` subcommand after `docker compose`
- Compose flag capture: everything between `compose` and `up`
- Config parsing: `docker compose [flags] config --format json`
- Multi-route registration, heartbeat, and cleanup
- `-n` flag overrides project name portion

## Compose Config JSON Structure

```go
type composeConfig struct {
    Name     string                      `json:"name"`
    Services map[string]composeService   `json:"services"`
}

type composeService struct {
    Ports []composePort `json:"ports"`
}

type composePort struct {
    Published string `json:"published"`
    Target    int    `json:"target"`
    Protocol  string `json:"protocol"`
}
```

## Lifecycle

1. `up` parses args, detects Docker Compose mode
2. Runs `docker compose config --format json` with captured compose flags
3. Parses config, extracts services with published ports
4. Registers a route for each service (`service.project` → `localhost:published_port`)
5. Starts `docker compose up` as child process (with process group)
6. Heartbeat goroutine sends heartbeats for all routes every 10s
7. On SIGINT/SIGTERM: forward to process group, deregister all routes, exit
8. If Docker Compose exits: deregister all routes, exit with same code

## What Doesn't Change

- DNS server (already handles all `*.test` queries)
- Reverse proxy (routes to whatever upstream is registered)
- SSL/cert generation (generates certs for any `.test` domain on demand)
- Single-app `up` codepath (entirely separate branch)
- Daemon (just receives routes via existing Unix socket API)

## Out of Scope

- Running paw-proxy inside a Docker container
- Docker label-based auto-discovery (watching Docker socket)
- Docker Compose v1 (`docker-compose` hyphenated binary)
- Dynamic port allocation (`0:3000` → random host port) — could be added later using Approach C (`docker compose ps`)
