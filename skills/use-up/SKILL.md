---
name: use-up
description: Use when starting dev servers, running web applications locally, launching local HTTP servers, or exploring projects that have web servers. Also use when the user asks to run a project, start a service, or mentions port conflicts or local HTTPS. This skill MUST be used any time you are about to run a command that starts an HTTP server — including npm run dev, bun dev, yarn dev, next dev, vite, go run, python manage.py runserver, flask run, rails server, cargo run, docker compose up, or any similar dev server command. Even if the user doesn't mention paw-proxy or `up`, always use this skill when a dev server is involved.
---

# Always Use `up` to Start Dev Servers

The user has `up` installed globally (from paw-proxy). It wraps dev server commands to provide automatic HTTPS, free port allocation, and friendly `.test` domain names. This eliminates port conflicts when running multiple projects simultaneously.

Every dev server command MUST be wrapped with `up`. This is how the user's environment works — it is not optional.

## How It Works

`up` finds a free port, registers that port with the paw-proxy daemon under a `.test` domain, runs the dev server on that port, and keeps the registration alive until the server stops. The proxy handles HTTPS termination, DNS, and certificate generation.

## Rules

1. **Always prefix dev server commands with `up`** — never run bare `npm run dev`, `bun dev`, `go run .`, `python manage.py runserver`, etc.
2. **Never set `PORT` explicitly** — `up` allocates a free port and sets the `PORT` environment variable automatically
3. **Never add `--port` flags** — same reason, `up` handles port allocation
4. **Never configure HTTPS/TLS in the dev server** — paw-proxy handles TLS termination
5. **Use `-n <name>` when the auto-detected name isn't ideal** — especially in monorepos, git worktrees, or directories with generic names
6. **Tell the user their app is at `https://<name>.test`** — never say `http://localhost:XXXX`
7. **Use `--restart` for crash-prone servers** — `up --restart <cmd>` auto-restarts on non-zero exit
8. **For Docker Compose** — use `up docker compose up` instead of bare `docker compose up`

## Usage Patterns

### Single app

```bash
up npm run dev          # name from package.json
up bun dev              # name from package.json
up -n myapp go run .    # explicit name (Go has no package.json)
up flask run
up rails server
up cargo run
up -n api python manage.py runserver
```

### Docker Compose

```bash
up docker compose up
up -n shop docker compose up
```

Docker Compose routes use the pattern `{service}.{project}.test` — for example, `https://frontend.myapp.test` and `https://api.myapp.test`.

### Monorepo with multiple services

Run each service in a separate shell with an explicit `-n` name:

```bash
# Shell 1 — from apps/frontend/
up -n frontend npm run dev

# Shell 2 — from apps/api/
up -n api go run .
```

Derive names from the subdirectory or package name.

## Environment Variables

`up` sets these automatically for the child process (single-app mode):

| Variable | Example | Purpose |
|----------|---------|---------|
| `PORT` | `54321` | Free port for the dev server |
| `APP_DOMAIN` | `myapp.test` | The `.test` domain |
| `APP_URL` | `https://myapp.test` | Full HTTPS URL |
| `HTTPS` | `true` | Signals HTTPS to frameworks |
| `NODE_EXTRA_CA_CERTS` | `~/.paw-proxy/ca.crt` | Trust the paw-proxy CA |

In Docker Compose mode, only `NODE_EXTRA_CA_CERTS` is set. Compose services use their own published ports.

## Name Conflicts

If the auto-detected name is already registered (common with git worktrees running the same project on different branches), `up` automatically falls back to the directory name. To avoid confusion, use `-n` with an explicit unique name.

## What to Tell the User

After starting a single-app server:
> "Your app is running at `https://myapp.test`"

After starting Docker Compose:
> "Your services are running at `https://frontend.myapp.test` and `https://api.myapp.test`"

## If `up` Is Not Found

If `up` is not in `$PATH`, tell the user:
> "`up` is not installed. Install paw-proxy with `brew install alexcatdad/tap/paw-proxy`, then run `sudo paw-proxy setup`."

Do NOT fall back to running bare dev server commands. The user expects `up` to be used.

## If Something Goes Wrong

If `up` reports the daemon is not running or `.test` domains don't resolve, suggest:
> "Run `paw-proxy doctor` to diagnose the issue."
