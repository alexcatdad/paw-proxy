# Skill Design: `use-up`

**Date:** 2026-03-16
**Status:** Draft
**Type:** Global Claude Code skill

## Problem

When Claude starts dev servers across different projects, it runs bare commands like `npm run dev` or `go run .`. This causes port conflicts when multiple projects are running simultaneously, and misses the HTTPS + automatic naming that paw-proxy provides via the `up` command.

## Solution

A global Claude Code skill (`~/.claude/skills/use-up.md`) that teaches Claude to always wrap dev server commands with `up` from paw-proxy. The skill is rigid — `up` is how the user's environment works, not a suggestion.

## Skill Identity

- **Name:** `use-up`
- **Location:** `~/.claude/skills/use-up.md`
- **Scope:** All projects (user-level, not project-level)
- **Type:** Rigid enforcement

## Trigger Conditions

### When to activate

- Claude is about to run a dev server command (`npm run dev`, `bun dev`, `go run .`, `python manage.py runserver`, `flask run`, `rails server`, `cargo run`, `next dev`, `vite`, etc.)
- Claude detects `docker-compose.yml` or `compose.yml` and is about to run `docker compose up`
- User asks to start, run, or launch a local web server
- User mentions port conflicts or HTTPS for local dev

### When NOT to activate

- Running tests, builds, linters, or other non-server commands
- Production deployments
- Projects without HTTP servers (CLI tools, libraries, scripts)

## Core Rules

1. **Always use `up` to start dev servers** — never run bare `npm run dev`, `bun dev`, `go run .`, etc.
2. **Don't specify `PORT`** — `up` allocates a free port and sets `PORT` automatically
3. **Use `-n` when the auto-detected name isn't ideal** — especially in monorepos, worktrees, or generic directory names
4. **For Docker Compose** — use `up docker compose up` instead of bare `docker compose up`
5. **Reference the app by its `.test` URL** — when telling the user their app is running, say `https://myapp.test` not `http://localhost:3000`
6. **Don't configure HTTPS in the dev server itself** — paw-proxy handles TLS termination

## Usage Patterns

### Single app (Node.js)

```bash
up npm run dev        # name auto-detected from package.json
up -n myapp bun dev   # explicit name
```

### Single app (Go/Python/Ruby/Rust)

```bash
up go run .
up -n api python manage.py runserver
up flask run
up rails server
up cargo run
```

### Docker Compose

```bash
up docker compose up
up -n shop docker compose up
```

### Monorepo (multiple services)

```bash
# Terminal 1 — from apps/frontend/
up -n frontend npm run dev

# Terminal 2 — from apps/api/
up -n api go run .
```

For monorepos, Claude should:
- Use `-n` to give each service a distinct name
- Run services in separate shells
- Derive names from subdirectory or package name

## What Claude should tell the user

After starting a dev server:
> "Your app is running at `https://myapp.test`"

NOT `http://localhost:XXXXX`.

## What Claude should NOT do

- Set `PORT=3000` or any explicit port
- Add `--port` flags to dev server commands
- Configure self-signed certs or HTTPS in the dev server config
- Run bare dev server commands without `up`

## Detection Heuristics

When exploring a project, Claude should recognize these signals that `up` is relevant:

### Strong signals (dev server project)

- `package.json` with a `dev` or `start` script
- `compose.yml` or `docker-compose.yml` with services exposing ports
- `manage.py` (Django)
- `Gemfile` with `rails` (Rails)
- `main.go` importing `net/http` or a web framework (gin, echo, fiber, chi)
- `Cargo.toml` with `actix-web`, `axum`, `rocket` dependencies
- `requirements.txt` or `pyproject.toml` with `flask`, `fastapi`, `django`

When these signals are present, Claude should proactively mention `up` if the user asks to run the project.

### No detection needed

- CLI tools, libraries, or packages without HTTP servers
- Static sites without a dev server
- Scripts or one-off utilities

## Environment Variables (reference)

`up` automatically sets these for the child process:

| Variable | Value | Purpose |
|----------|-------|---------|
| `PORT` | Auto-allocated free port | Dev server listens here |
| `APP_DOMAIN` | `myapp.test` | The `.test` domain name |
| `APP_URL` | `https://myapp.test` | Full HTTPS URL |
| `HTTPS` | `true` | Signal to frameworks |
| `NODE_EXTRA_CA_CERTS` | Path to CA cert | Trust paw-proxy CA for outbound HTTPS |

## Assumptions

- `up` is installed globally and available in `$PATH` (via Homebrew)
- `paw-proxy` daemon is running (installed via `sudo paw-proxy setup`)
- The `.test` TLD is resolved by paw-proxy's DNS server
