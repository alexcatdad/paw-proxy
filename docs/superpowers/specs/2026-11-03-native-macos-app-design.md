# Design: Native macOS App (Paw Proxy.app)

**Date:** 2026-11-03
**Status:** Draft — awaiting review
**Scope:** macOS app identity, app-owned daemon registration, menu-bar route view, retirement of the `_paw.test` web dashboard, cask distribution
**Reference implementations:** Scratchpad (`../scratchpad/macos`, always-on service + menu-bar app) and Tapas (`../tapas-archiver/macos`, migration from legacy LaunchAgents)

## Problem

On macOS today, `sudo paw-proxy setup` writes `~/Library/LaunchAgents/dev.paw-proxy.plist`, pointing at an unsigned Homebrew binary (`/opt/homebrew/bin/paw-proxy run`), and loads it with `launchctl bootstrap`.

- macOS has no signed app to attribute the background item to, so it appears in Login Items under an anonymous or third-party name. Permissions and approval cannot be retained across upgrades.
- There is no native surface to see what is routed. The only view is the `https://_paw.test` web dashboard, which lives inside the proxy's request path.
- Distribution ships unsigned, un-notarized binaries through a formula.

Scratchpad and Tapas have already solved this pattern. Tapas's macOS notes explicitly defer Paw Proxy: its app "should retain its own identity, existing proxy/CLI behavior and private configuration, and migrate its current launch agent without two simultaneous proxy instances." This spec is that work.

## Decisions (confirmed 2026-11-03)

| Topic | Decision |
|---|---|
| Web dashboard | **Retired.** `_paw.test`, `internal/dashboard` and request metrics are removed. The menu-bar app shows routes and health. |
| Platforms | **Apple Silicon only, macOS 13+** for the app (the `SMAppService` floor; matches Scratchpad). Intel macOS is dropped and must be announced in the changelog. Linux stays CLI-only. |
| Distribution | **A cask replaces the formula on macOS.** The cask links `paw-proxy` and `up` onto PATH. The formula stays for Linux only and points macOS users to the cask. |

## Goals

1. A signed, notarized `Paw Proxy.app` that owns the daemon's LaunchAgent through `SMAppService`, so macOS attributes it to Paw Proxy.
2. An optional menu-bar icon that lists the current route mappings and daemon health. The proxy runs whether or not the tray is open.
3. A one-way, rollback-safe migration from `dev.paw-proxy` that never runs two proxies at once.
4. The CLI keeps working (`up`, `paw-proxy setup|status|logs|doctor|uninstall`), including how it is invoked.

## Non-goals

- Changing proxy, DNS, TLS or routing behavior.
- A request inspector or traffic feed in the tray. `paw-proxy logs` covers that need.
- Sparkle or in-app auto-update. Homebrew owns updates.
- An Intel build, or a Linux GUI.
- Reworking the release pipeline. Local releases via release-tools already landed in #184; this spec only adds an app target and a cask entry.

## Architecture

### Bundle layout

```
Paw Proxy.app/
  Contents/
    Info.plist                     CFBundleIdentifier com.alexcatdad.paw-proxy, LSUIElement true, LSMinimumSystemVersion 13.0
    MacOS/PawProxy                 SwiftUI menu-bar app (MenuBarExtra)
    Helpers/paw-proxy              Go daemon + CLI (CGO on, for launch_activate_socket)
    Helpers/up                     Go command wrapper
    Library/LaunchAgents/
      com.alexcatdad.paw-proxy.daemon.plist
    Resources/AppIcon.icns, tray template PNGs (1x/2x)
```

- **Bundle ID:** `com.alexcatdad.paw-proxy`. The dev build uses `com.alexcatdad.paw-proxy.dev` (see Development builds).
- **Daemon label:** `com.alexcatdad.paw-proxy.daemon`, a fresh label. `dev.paw-proxy` is not reused. Tapas found that Background Task Management rejected reusing a label macOS had already attributed to another identity, while a fresh label registered cleanly.
- The Swift package follows Scratchpad's split: an executable target `PawProxy`, a library `PawProxyNative` holding all testable logic, and a test target.

### Daemon agent plist

```xml
<key>Label</key><string>com.alexcatdad.paw-proxy.daemon</string>
<key>BundleProgram</key><string>Contents/Helpers/paw-proxy</string>
<key>ProgramArguments</key><array><string>paw-proxy</string><string>run</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ProcessType</key><string>Interactive</string>
<key>Sockets</key>  <!-- http 127.0.0.1:80, https 127.0.0.1:443, identical to today's template -->
```

- `KeepAlive` stays `true`, unlike Scratchpad's `false`. The proxy is infrastructure: if it dies, every `*.test` URL breaks, and the user has no reason to stop it other than disabling it. "Off" means unregistering the agent, not a persistent stop flag.
- `ProcessType Interactive` avoids background throttling of request handling.
- Logs keep their existing path (`~/Library/Logs/paw-proxy.log`), written by the daemon itself. The plist sets no `StandardOutPath`.

> [!warning] Unverified assumption — Phase 0 spike
> `SMAppService.agent` plists are launchd plists, and the `Sockets` key should be honored. That has not been observed for a bundled agent. If it is not honored, the daemon already falls back to binding 127.0.0.1:80/443 directly ([daemon.go:333](../../../internal/daemon/daemon.go)); macOS 10.14+ lets unprivileged processes bind those ports. The spike decides which path the plist uses, and the release does not depend on socket activation.

### Process model

| Process | Started by | Lifetime | Role |
|---|---|---|---|
| `paw-proxy run` (daemon) | launchd, via the app's `SMAppService.agent` registration | Always on while registered (`KeepAlive`) | Proxy, DNS, API socket |
| `PawProxy` (tray) | User, or login if "Show in menu bar at login" is on (`SMAppService.mainApp`) | Optional | Shows status and routes; registers, unregisters and restarts the daemon |
| `paw-proxy <cmd>` / `up` (CLI) | User shell, via cask `binary` links into `Contents/Helpers` | Per command | Unchanged behavior |

Quitting the tray never stops the proxy. The menu says so ("Quit menu bar icon — proxy keeps running"), as Scratchpad's does.

### Tray ↔ daemon communication

No new IPC protocol. The tray uses the existing HTTP API on the private Unix socket (`~/Library/Application Support/paw-proxy/paw-proxy.sock`, mode 0600), which `up` already uses:

- `GET /health` → `{status, version, uptime}` (already exists). Add a `build` field so the tray can detect an outdated daemon after an update.
- `GET /routes` → `[{name, upstream, dir, registered, lastHeartbeat}]` (already exists).

Polling follows Scratchpad: every 10 s for the icon tone, an immediate refresh when the menu opens, and fast polling (250 ms × 20) after a restart. Scratchpad's `ServiceLifecycleObservationRevision` idea is reused in a reduced form: a counter bumped on each user action drops stale in-flight replies, so a "Restarting" state is not overwritten by a reply from before the restart.

The tray does not need its own socket permissions. It runs as the same user and connects to a user-owned 0600 socket. Peer-UID checking on the daemon side is tracked separately ([#173](https://github.com/alexcatdad/paw-proxy/issues/173)).

### One status model

A single `StatusPresentation(health:routes:registration:bundleVersion:)` drives the menu-bar icon and the menu header, as in Scratchpad. It checks these in order, and the first match wins:

1. **Setup incomplete:** CA or `/etc/resolver/test` missing, read from files the CLI writes. Action: "Run setup…" copies `sudo paw-proxy setup` to the clipboard and explains why. The app never asks for admin rights itself.
2. **Needs approval:** `SMAppService` status is `.requiresApproval`. Action: "Open Login Items" (`SMAppService.openSystemSettingsLoginItems()`).
3. **Not registered:** status `.notRegistered` or `.notFound`. Action: "Turn on proxy".
4. **Not responding:** registered, but the socket does not answer. Action: "Restart proxy" or "Open logs".
5. **Restart needed:** the running daemon's `version`/`build` differs from the bundle's. This happens after `brew upgrade`, which replaces the bundle while the old binary keeps running. Action: "Restart to finish update". Builds marked `dev` never trigger this state.
6. **Running:** shows the route count. The icon is a template image; tones other than ok add a badge.

### Menu contents (`MenuBarExtra`, window style)

- **Header:** status line, version, uptime.
- **Route list** (the "current maps"): `name.test → 127.0.0.1:PORT`, the project directory's basename, and the heartbeat age. A route whose heartbeat is older than half the registry timeout is dimmed. Clicking a row opens `https://name.test`; the secondary action copies the URL. An empty state explains `up <command>`.
- **Actions:** Restart proxy, Open logs (Console or the log file), Settings (Show in menu bar at login; Turn proxy on/off), Quit menu bar icon.
- **Restart:** `launchctl kickstart -k gui/<uid>/com.alexcatdad.paw-proxy.daemon`. `SMAppService` has no restart call, and kickstart works on a registered user agent.

### Registration ownership

`SMAppService` calls must come from the app process so macOS attributes them to Paw Proxy (the Tapas lesson). Privileged steps stay in the CLI.

- **`sudo paw-proxy setup`** keeps the root-only work: CA creation and trust, `/etc/resolver/test`, Firefox NSS. Then, as the real user (the existing `SUDO_USER` handling in `launchctlAsUser`), it launches the app through Launch Services with a one-shot request:
  - `open -g -b com.alexcatdad.paw-proxy --args --request <file>`, where the request and reply are private files under the support directory (Tapas's `DesktopRequest` pattern, with no listener or broker).
  - The app performs the migration and registration (next section), writes the reply, and exits if it was not already running.
  - The CLI waits for the reply (with a timeout) and prints the outcome, including "approve in Login Items" when needed.
- **First launch of the app** with setup already complete and the agent not registered offers "Turn on proxy". This covers people who install the cask after setting up by hand.
- **The CLI finds its bundle** through `os.Executable()` and `filepath.EvalSymlinks`, so it works through the cask's PATH links. Run outside a bundle (`go build`, Linux), it behaves as today: Linux keeps systemd, and macOS errors with "install the Paw Proxy cask" unless `--legacy-launchagent` is passed. That flag is kept only for development and is removed after one release cycle.

### Migration from `dev.paw-proxy`

The migration runs inside the app when it receives a `register` request and finds `~/Library/LaunchAgents/dev.paw-proxy.plist` or a loaded `gui/<uid>/dev.paw-proxy`. It never leaves both agents loaded, and it rolls back on failure.

1. **Preflight:** verify the app's own signature (`codesign --verify --deep --strict`). Refuse if it is invalid, except in dev builds, which never register (see below).
2. **Backup:** copy the legacy plist to `~/Library/Application Support/paw-proxy/migration/dev.paw-proxy.plist` (0600) and record its loaded state.
3. **Stop legacy:** `launchctl bootout gui/<uid>/dev.paw-proxy`. Wait until the API socket stops answering and 127.0.0.1:443 is free, polling at 250 ms with a 10 s timeout. `bootout` releases launchd's socket reservations; `unload` does not.
4. **Register new:** `SMAppService.agent(plistName:).register()`.
5. **Verify:** within 10 s, `/health` answers and the daemon's executable path (via `proc_pidpath` on the PID from `launchctl print`) is inside the bundle. `.requiresApproval` counts as a pending success: the legacy agent stays booted out, the plist backup is kept, and the CLI and tray say approval is pending.
6. **Commit:** delete the legacy plist and leave the backup in place for one release.
7. **Roll back** on any failure in steps 4–5: `unregister()`, restore the plist, `launchctl bootstrap gui/<uid>` it, confirm `/health` answers again, and report the original error.

The CA, resolver file, support directory and logs are unchanged by the migration. Their paths stay identical.

### Uninstall

- **`paw-proxy uninstall`** sends an `unregister` request to the app (the agent and, if set, the login item for the tray), then removes the CA trust, resolver file and support directory as today. If the app is missing, it falls back to `bootout` of both labels.
- **The cask** has `uninstall quit: "com.alexcatdad.paw-proxy"` and a caveat to run `sudo paw-proxy uninstall` before `brew uninstall --cask`. The cask does not delete `/etc/resolver/test` or keychain entries itself. That stays an explicit user action, matching how Scratchpad's cask avoids system-level side effects.
- A `zap` stanza removes `~/Library/Application Support/paw-proxy` and the log file. It does not touch system files.

### Upgrades

Homebrew 7 rejects Ruby `preflight`/`postflight` hooks, and restarting from them is unsafe while Homebrew can still roll back (Scratchpad's decision). So:

- `brew upgrade` replaces the bundle. The running daemon keeps the old binary until restarted.
- The tray shows "Restart to finish update" (status 5), and `paw-proxy status` prints the same version mismatch.
- The cask caveat tells users to run `paw-proxy restart`. That is a new small subcommand doing the same kickstart as the tray.
- `KeepAlive` means any crash or logout also picks up the new binary.

### Development builds

The approach follows Scratchpad's `dev-app.sh`, tightened against its known gap: Scratchpad's dev app shares the production agent label.

- `macos/Scripts/dev-app.sh` builds `macos/.build/dev/Paw Proxy Dev.app` with bundle ID `com.alexcatdad.paw-proxy.dev`, ad-hoc signed, and **with no `Contents/Library/LaunchAgents`**, as Tapas does for its dev app.
- The script refuses to overwrite a bundle whose ID does not end in `.dev`.
- The dev tray connects to whatever daemon owns the socket and disables registration actions with the label "Dev build — registration disabled". A "DEV" badge appears in the menu bar.
- This lets the tray UI iterate against the real installed daemon without touching Login Items.

## Web dashboard retirement

Remove:

- `internal/dashboard/` (including `metrics.go` and the static assets).
- The `_paw` intercept and `metrics.Record` calls in `internal/daemon/daemon.go`.
- The dashboard sections in `README.md` and the feature card in `docs/index.html`.
- `docs/plans/2026-02-19-web-dashboard-plan.md`, which is marked superseded and kept for history.

`_paw.test` will then return the normal "no route" error page. Because the name `_paw` cannot be registered (route names must start with an alphanumeric), no user route can take it over.

This is a user-visible removal: changelog entry and Decision Log entry. Linux users lose the dashboard and get no tray. `paw-proxy status` and `paw-proxy logs -f` are the replacements, and the README must say so.

## Packaging and signing

`macos/Scripts/package-app.sh` follows Scratchpad's script:

1. Validate `APP_VERSION` and `BUILD_NUMBER`. Build `PawProxy` with SwiftPM (arm64). Build `paw-proxy` and `up` with `CGO_ENABLED=1 GOARCH=arm64` and `-ldflags -X …Version -X …Build`.
2. Assemble the bundle in a private staging directory. Refuse to replace a destination with a different bundle ID.
3. Run `plutil -lint` on both plists, then `validate-bundle.sh`: helpers are executable, `paw-proxy version` and `up --version` match the Info.plist version and build, the agent plist's `BundleProgram` exists, and the label is not `dev.paw-proxy`.
4. Sign: `paw-proxy` and `up` with `--options runtime --timestamp`, then the outer app last so its seal covers the finished helpers, then `codesign --verify --deep --strict`. No entitlements are needed (no keychain or sandbox).
5. Swap into the destination atomically, with rollback.

Notarization, stapling, zipping (`ditto`), the draft release with SHA-256 verification, and the cask update belong to the local release flow already adopted in #184 (`release.json` + `task release` via `reltool`). The app adds a `signing.macos` block and a second tap entry (`taps[]`, kind `cask`, darwin only), both supported by release-tools v0.2.0. Until that release change lands, `package-app.sh` produces local candidates only and nothing is published.

## Testing

**Automated (CI on `macos-15`, and locally):**

- Swift `PawProxyNative` tests:
  - status precedence and update-state comparison, including the `dev` exemption
  - route-row formatting and heartbeat dimming
  - the stale-reply revision counter
  - request/reply file parsing, which rejects unknown ops, oversize input and wrong owner/mode
  - a socket client test against a Go fixture daemon started on a temp socket (gated by an env var, as Scratchpad's `ServiceProcessSmokeTests` is)
  - the migration state machine (it runs in the app) with a fake `launchctl` runner and a fake registrar, covering the success path, `requiresApproval`, a register failure followed by rollback, and a verify timeout followed by rollback
- Go tests:
  - bundle discovery through symlinks
  - the `--legacy-launchagent` gate
  - the CLI side of the request/reply exchange: writing the request, timing out, reporting each reply outcome
  - removal of the dashboard intercept (`_paw.test` returns the not-found page)
- Script tests: `test-package-app.sh` (unsigned layout, refusal to overwrite a different bundle ID) and `test-dev-app.sh` (no LaunchAgents directory in the dev bundle).

**Installed acceptance (human, results recorded in Scratchpad; never inferred from plist contents):**

- [ ] A bundled agent with `Sockets` receives activated sockets (Phase 0), or the direct-bind fallback is chosen.
- [ ] Login Items lists "Paw Proxy" with its icon, not an anonymous item.
- [ ] Migration from a live `dev.paw-proxy` leaves exactly one proxy, with no window where both run. Rollback restores the legacy agent when registration is forced to fail.
- [ ] Login, crash (`kill -9`) and `brew upgrade` each leave exactly one daemon. The tray detects the update, and Restart picks up the new binary.
- [ ] Tray quit leaves the proxy running. "Show in menu bar at login" works.
- [ ] `paw-proxy uninstall` and then `brew uninstall --cask` leave no agent, login item, resolver file or CA trust.
- [ ] Clean-machine install: cask → `sudo paw-proxy setup` → `up` → https URL works.

## Phasing

| Phase | Deliverable | Depends on |
|---|---|---|
| 0 | Spike: a minimal signed bundle with a `Sockets` agent registered via `SMAppService`. Record the result in the Decision Log. | Developer ID on this Mac |
| 1 | Go prep: dashboard removal; `build` in `/health`; bundle discovery; `paw-proxy restart`; the request/reply client in the CLI; the `--legacy-launchagent` gate | — |
| 2 | `macos/` package: tray app, status model, plists, `package-app.sh`, `dev-app.sh`, validators, Swift tests | 0 |
| 3 | Registration and migration inside the app; `setup`/`uninstall` integration; migration tests | 1, 2 |
| 4 | Distribution: app target, signing and cask entry in `release.json`; formula restricted to Linux with a deprecation notice; Intel-drop announcement | 3 |
| 5 | README, `docs/index.html`, man pages; installed acceptance run, results recorded in Scratchpad | 4 |

Phases 1 and 2 can proceed in parallel; they touch disjoint files.

## Open questions

1. App display name: "Paw Proxy" (matching Tapas/Scratchpad's title case) or "paw-proxy"? This spec assumes **Paw Proxy**.
2. Should `paw-proxy setup` on macOS also turn on "Show in menu bar at login" by default, or leave the tray strictly opt-in? This spec assumes **opt-in**: the tray is optional and the proxy is not.
3. Formula `Formula/paw.rb`: alias, keep or remove?
