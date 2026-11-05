# Plan: Native macOS App (Paw Proxy.app)

**Date:** 2026-11-03
**Design:** [2026-11-03-native-macos-app-design.md](../specs/2026-11-03-native-macos-app-design.md)
**Memory:** Scratchpad project `paw-proxy`. Read the ADR "macOS daemon is owned by a signed Paw Proxy.app via SMAppService", the decisions on dashboard retirement and cask distribution, and the "No repo-local knowledge base" constraint. Record each phase's outcome there.
**References:** `../scratchpad/macos`, `../tapas-archiver/macos` and `../tapas-archiver/src/desktop.ts`, plus `../release-tools/docs/contracts.md` (v0.2.x).

Each phase ships as its own PR, following the repo PR workflow (CLAUDE.md). A phase may start only when the phases it depends on have merged. Phases 1 and 2 touch disjoint files and can run in parallel.

```
P0 spike ──► P2 macOS package ─┐
P1 Go prep ────────────────────┼─► P3 registration + migration ─► P4 distribution ─► P5 docs + acceptance
```

## Before starting

Local `main` has diverged from origin: it is 3 commits behind (#181, #182, #184) and carries 3 unpushed commits (the use-up skill: `f6c2ee1`, `a6ca914`, `d2d011d`).

- Cut every branch for this plan from `origin/main`, not local `main`.
- Land the use-up commits on their own branch and PR (`git rebase origin/main` onto a `feat/use-up-skill` branch). That is a separate decision; do not fold them into this work.
- Toolchain: Go per `go.mod` on origin (1.26.10+); Swift 6 with the Command Line Tools; `task`; `reltool` pinned through the Taskfile (`RELTOOL_TAG`).

---

## Phase 0: Spike — do bundled agents receive launchd sockets?

**Goal:** Decide whether the daemon plist keeps `Sockets` or relies on the daemon binding 127.0.0.1:80/443 itself.
**Branch:** none. The spike lives in the session scratchpad and is never committed.
**Requires:** a Developer ID Application identity on this Mac (`security find-identity -v -p codesigning`). A human runs the GUI approval steps.

Steps:

1. Build a minimal bundle `Spike.app` (bundle ID `com.alexcatdad.paw-proxy.spike`):
   - `Contents/MacOS/Spike`: a ~40-line Swift binary taking the argument `register` or `unregister`, which calls `SMAppService.agent(plistName: "com.alexcatdad.paw-proxy.spike.plist")` and prints its status.
   - `Contents/Helpers/paw-proxy`: the current daemon built from `origin/main` (CGO on).
   - `Contents/Library/LaunchAgents/com.alexcatdad.paw-proxy.spike.plist`: the spec's daemon plist, but with unused ports 8080/8443 so it cannot collide with the running `dev.paw-proxy`.
2. Sign the helper, then the app (`--options runtime --timestamp`). Copy it to `/Applications/`, run `register`, and approve in Login Items if prompted.
3. Observe and record:
   - The daemon log line "using launchd socket activation" versus "falling back to direct binding".
   - `launchctl print gui/$(id -u)/com.alexcatdad.paw-proxy.spike`: shows the sockets and the program path inside the bundle.
   - Login Items shows the app's name and icon.
4. Run `unregister` and delete the spike bundle.

**Result (2026-11-03):** a Developer ID-signed spike registered straight to `enabled` (no approval prompt) and `launch_activate_socket` returned the launchd-bound socket inside the bundle. **The daemon plist keeps `Sockets`.** Recorded in Scratchpad. Not yet observed: the Login Items label (needs a human), ports 80/443 under `SMAppService`, and behavior on the macOS 13 floor.

**Exit:**
- A Scratchpad `record_finding` with the outcome, macOS version and limitations.
- If sockets are not delivered: edit the spec's plist section to drop `Sockets` and rely on direct binding. P1 then adds a test proving direct bind to 127.0.0.1:80 works unprivileged on macOS 13+, and the CI job can run that on a `macos-15` runner.

---

## Phase 1: Go preparation

**Branch:** `feat/native-app-go-prep` · **Depends on:** — · **PR title:** `feat!: retire _paw.test dashboard and prepare CLI for the macOS app`

### 1.1 Retire the web dashboard

- Delete `internal/dashboard/` (all files, including `static/`).
- `internal/daemon/daemon.go`:
  - remove the `metrics`/`dash` fields and their construction in `New`
  - remove the `_paw` intercept in `handleRequest`
  - remove both `metrics.Record` calls (`statusCapture` stays; WebSocket support depends on it)
- `internal/daemon/daemon_test.go`: add `TestPawHostReturnsNotFound`. A request with `Host: _paw.test` must get the standard not-found page via `serveNotFound`.
- `internal/help/*`: remove any dashboard text. Then run `go run ./cmd/gen-man` to regenerate `man/`.
- `docs/plans/2026-02-19-web-dashboard-plan.md`: add a superseded banner at the top linking the design spec.

### 1.2 Version and build identity

- Add a `Build` variable alongside `Version`:
  - `internal/api/server.go`: `var Build = "dev"`
  - the `/health` payload gains `"build": Build`
  - `internal/api/server_test.go`: assert the field is present
- `cmd/paw-proxy/main.go`: `paw-proxy version` prints `paw-proxy version X (build N)`.
- `cmd/up/main.go`: `-version` prints the same format.
- *Implemented:* `reltool` exposes no build number, so the Taskfile is unchanged and `Build` stays empty for standalone binaries (version output and release validation are unchanged). `macos/Scripts/package-app.sh` (P2) sets `main.build` and `api.Build` to `CFBundleVersion`.

### 1.3 Bundle discovery

- New package `internal/appbundle` (darwin build tag, plus a stub returning `ErrNotInBundle` elsewhere):
  - `Locate() (Bundle, error)`: `os.Executable()` → `filepath.EvalSymlinks` → walk up to `*.app/Contents/Helpers/<exe>` → read `Contents/Info.plist` (`CFBundleIdentifier`, `CFBundleShortVersionString`, `CFBundleVersion`) with `plutil -convert json -o -` or a minimal XML decode. No new dependency.
  - `Bundle{Path, ID, Version, Build}`; `IsDev()` reports whether the ID ends in `.dev`.
- Tests: synthetic bundles under `t.TempDir()`:
  - a direct helper path, a symlink chain like `/opt/homebrew/bin/paw-proxy → …/Helpers/paw-proxy`, a non-bundle path, and a malformed plist.

### 1.4 `paw-proxy restart`

- `cmd/paw-proxy/main.go`: add `case "restart"`. It runs `launchctl kickstart -k gui/<uid>/<label>`, where the label is `com.alexcatdad.paw-proxy.daemon` when `appbundle.Locate()` succeeds and `dev.paw-proxy` otherwise.
- Afterwards it polls `/health` (250 ms × 40) and prints the new version and build.
- Linux: `systemctl --user restart paw-proxy` (match the unit name in `setup_linux.go`).
- Help text and the man page are regenerated.

### 1.5 Desktop request client (CLI side)

- New package `internal/desktop`:
  - `Request{Version:1, ID, Op}` with ops `register`, `unregister` and `status`.
  - `Reply{Version, ID, Outcome, Detail}` with outcomes `registered`, `requiresApproval`, `unregistered`, `notRegistered`, `failed` and `rolledBack`.
  - *Implemented:* the opener runs `open -n -g -a <bundle path> --args --request <file>`. `-n` is required because `open` drops `--args` for an already-running app, and `-a <path>` targets the exact bundle the CLI runs from. Shared vectors live in `internal/desktop/testdata/vectors.json`.
  - `Send(ctx, bundle, req) (Reply, error)`:
    1. Write the request (0600) to `<support>/requests/<id>.json`, with the directory at 0700.
    2. Run `open -g -b <bundleID> --args --request <path>`, as `SUDO_USER` when running under sudo. Reuse the logic of `launchctlAsUser` and generalize it as `runAsRealUser` in `internal/setup/helpers.go`.
    3. Poll for `<id>.reply.json` until the timeout (default 60 s, which allows for an approval prompt).
    4. Validate version, ID, ownership and mode, then remove both files.
  - Oversize files (>64 KiB), a wrong owner or a wrong mode are rejected.
- Tests: a fake opener that writes replies, covering each outcome, a timeout, ID mismatch, a 0644 reply file, and leftover-file cleanup.

### 1.6 Gate the legacy LaunchAgent

- `internal/setup/config.go`: add `LegacyLaunchAgent bool`.
- `cmd/paw-proxy/main.go`: parse `--legacy-launchagent` in `cmdSetup`.
- `internal/setup/setup_darwin.go` `Run`:
  - Keep the CA/resolver/NSS steps.
  - Replace the unconditional `installLaunchAgent` with:
    - in a bundle → `desktop.Send(register)`, printing the outcome (for `requiresApproval`: "Approve Paw Proxy in System Settings › General › Login Items")
    - not in a bundle with `--legacy-launchagent` → `installLaunchAgent`, as today
    - neither → an error saying "install the cask (`brew install --cask alexcatdad/tap/paw-proxy`) or pass --legacy-launchagent"
- `uninstall_darwin.go`: in a bundle, `desktop.Send(unregister)` first. Always also run `bootout` on both labels and remove `~/Library/LaunchAgents/dev.paw-proxy.plist` if present, so leftovers are cleaned whichever path installed them.
- `internal/setup/setup_darwin_test.go`: table tests for the three branches, using injected `locate`/`send`/`install` functions. Introduce a small struct of function fields so the tests need neither root nor launchctl.
- *Implemented with a transition gate:* the "neither" branch is controlled by `appRequired` in `internal/setup/daemon_install_darwin.go`, which is **false** until the cask ships, so formula installs keep writing the legacy agent. P4 flips it (a test guards against flipping early) and adds `--legacy-launchagent` to `lifecycle-tests.sh` and the CI macOS job at the same time.

**Verify:** `task check`, `go build ./cmd/paw-proxy ./cmd/up`, and the lifecycle tests on the macOS CI runner.

**Changelog:** under `### Removed`, add "`_paw.test` dashboard". Under `### Added`, add `paw-proxy restart` and the build number in `version`.

---

## Phase 2: macOS package (tray app, unsigned candidate)

**Branch:** `feat/native-app-macos-package` · **Depends on:** P0 (the plist shape) · **PR title:** `feat: add Paw Proxy menu-bar app package`

### 2.1 Swift package layout (`macos/`)

```
macos/Package.swift                     platforms: .macOS(.v13); products PawProxy (exe); targets PawProxy, PawProxyNative, PawProxyNativeTests
macos/Sources/PawProxyNative/
  DaemonClient.swift                    HTTP/1.1 over a Unix socket (POSIX socket, `Connection: close`, 1 MiB cap, 5 s timeout); GET /health, GET /routes
  Models.swift                          Health {status, version, build, uptime}; Route {name, upstream, dir, registered, lastHeartbeat}
  StatusPresentation.swift              precedence from the design spec: setupIncomplete > needsApproval > notRegistered > notResponding > restartNeeded > running
  UpdateState.swift                     bundle version/build vs daemon; "dev" on either side → current
  RouteRow.swift                        "name.test → 127.0.0.1:PORT", dir basename, heartbeat age, isStale (age > timeout/2)
  ObservationRevision.swift             a counter bumped on user intent; replies carrying an older revision are dropped
  SetupProbe.swift                      CA file + /etc/resolver/test presence (read-only)
  AppIdentity.swift                     isDevelopment from bundle ID suffix
  Paths.swift                           support dir, socket path, log path (mirror of internal/paths/paths_darwin.go)
macos/Sources/PawProxy/
  PawProxyApp.swift                     @main; MenuBarExtra(.window); no Dock icon (LSUIElement)
  AppModel.swift                        @MainActor ObservableObject; 10 s timer + refresh on open; drives StatusPresentation
  MenuView.swift                        header, route list (open URL / copy), actions, "Quit menu bar icon — proxy keeps running", DEV badge
macos/Tests/PawProxyNativeTests/        one test file per Native source
macos/Resources/
  Info.plist                            com.alexcatdad.paw-proxy, LSUIElement, LSMinimumSystemVersion 13.0
  com.alexcatdad.paw-proxy.daemon.plist per spec (Sockets kept or dropped per P0)
  AppIcon.icns, TrayIcon.png, TrayIcon@2x.png (template images, 18 pt)
```

- Copy the shape of Scratchpad's `IPCClient` tests, not its protocol: paw-proxy speaks HTTP, not length-prefixed JSON.
- `DaemonClientTests` starts a Go fixture: `go run ./internal/api/testfixture -socket <tmp>`. Add that tiny `main` serving a canned registry. The test is gated by `PAWPROXY_TEST_DAEMON=1`, like Scratchpad's `SCRATCHPAD_TEST_SERVICE`, and the CI job sets it so the test is mandatory there.
- Registration actions are stubbed behind a `Registrar` protocol in this phase (a no-op implementation). P3 supplies the real one.

### 2.2 Scripts (`macos/Scripts/`)

- `package-app.sh DEST`: steps 1–5 of the spec's "Packaging and signing" section.
  - Without `SIGNING_IDENTITY` it ad-hoc signs and prints "candidate only".
  - It refuses a destination with a different bundle ID and swaps into place atomically.
  - Go helpers are built with the Taskfile's LDFLAGS (`Version`/`Build`) and `CGO_ENABLED=1 GOARCH=arm64`.
- `validate-bundle.sh APP`:
  - `plutil -lint`, and helpers are executable
  - `paw-proxy version` and `up -version` match `CFBundleShortVersionString`/`CFBundleVersion`
  - the agent plist's `BundleProgram` exists, its label is `com.alexcatdad.paw-proxy.daemon`, and no `dev.paw-proxy` string appears anywhere in the bundle
  - the dev variant asserts there is no `Contents/Library/LaunchAgents`
- `dev-app.sh [--run]`: builds `macos/.build/dev/Paw Proxy Dev.app` with ID `com.alexcatdad.paw-proxy.dev` and no LaunchAgents, ad-hoc signed. It refuses a non-`.dev` destination. `--run` TERMs only that exact executable path, then opens it.
- `test-package-app.sh` and `test-dev-app.sh`: run against temp destinations. They cover layout, the wrong-ID refusal, validator failures on a tampered plist or version mismatch, and the dev bundle having no agent.

### 2.x Implementation notes

- `DaemonClient` sends HTTP/1.0, so Go's server ends the body by closing the connection instead of using chunked encoding. The live test covers a response over 2 KiB.
- The menu-bar icon is an SF Symbol (`pawprint.fill`, plus warning variants). The app icon is generated by `macos/Scripts/make-icon.swift`, so there are no template PNGs to maintain.
- `StatusPresentation` adds a "running from the old LaunchAgent" state: healthy daemon, agent not registered, with action `turnOn`. Until P3, `turnOn` only copies `sudo paw-proxy setup`. Registering directly would compete with the legacy agent for ports 80/443.
- `test-package-app.sh` also covers the dev build, so there is no separate `test-dev-app.sh`.
- Package scripts build the Go helpers with `go build -C <repo>` and `-trimpath`.

### 2.3 Taskfile and CI

- Taskfile: add `macos:test` (`swift test --package-path macos`), `macos:package` and `macos:dev`.
- `.github/workflows/ci.yml`: add a `macos-app` job on `macos-15`, path-filtered to `macos/**`, `internal/api/**` and `cmd/**`. It runs `PAWPROXY_TEST_DAEMON=1 task macos:test`, `test-package-app.sh` and `test-dev-app.sh`, and uploads logs.
- `.gitignore`: add `macos/.build/` and `macos/build/`.

**Verify:** `task check`, `task macos:test`, both script tests, and a manual `task macos:dev -- --run` showing live routes from the currently installed daemon.

---

## Phase 3: Registration and migration

**Branch:** `feat/native-app-registration` · **Depends on:** P1, P2 · **PR title:** `feat: register the daemon from Paw Proxy.app`

### 3.1 Native registration (`PawProxyNative`)

*Simplified during implementation (owner, 2026-11-04): paw-proxy is a dev tool, so switching agents is "remove the old one, register the new one". No backup, port wait, verify loop or rollback. If registration fails, the reply says to run `sudo paw-proxy setup --legacy-launchagent`.*

- `Registrar` (exists from P2) wraps `SMAppService.agent`; `LoginItem`/`MainAppLoginItem` wraps `.mainApp` for "Show in menu bar at login". The dev build injects `DisabledRegistrar`.
- `LaunchctlRunner`/`SystemLaunchctl`: `pid`, `bootout`, `bootstrap`, `kickstart` on `gui/<uid>`.
- `DaemonAgent`:
  - `turnOn()`: boot out `dev.paw-proxy` if loaded, delete its plist, then `register()` unless the agent is already enabled. Outcomes: `registered`, `requiresApproval`, or `failed` with the fallback hint.
  - `turnOff()` and `status()` map the registrar state to reply outcomes.
- `DaemonAgentTests` cover a clean install, legacy removal before register, approval, register failure, an agent missing from the bundle, idempotence, and the dev build touching nothing.

### 3.2 Request handling in the app

- `DesktopRequest.swift` validates the request file: inside `<support>/requests`, named `<id>.json`, a regular file owned by the current UID, mode 0600, ≤64 KiB, version 1, known op, and an ID matching the file name. Swift tests read the same `internal/desktop/testdata/vectors.json` as Go.
- `Entry.swift` is the process entry point: with `--request <path>` it handles the request and exits **before any SwiftUI scene exists**, so an `open -n` instance never adds a second menu-bar icon. A request that fails validation still gets a `failed` reply, but only when its path is a well-formed request in the requests directory.
- Menu actions: "Turn on proxy" (`DaemonAgent.turnOn`), "Turn proxy off", "Restart proxy" (`kickstart -k`), "Open Login Items" in the approval state, and a "Show in menu bar at login" checkbox.

### 3.3 Go integration test

- `internal/desktop`: add a test driving `Send` against a fake opener that executes the Swift test vectors' expected replies, which keeps the two implementations aligned.
- Make sure `lifecycle-tests.sh` still passes with `--legacy-launchagent`.

**Verify:** `task check` and `task macos:test`. Then a manual dev-machine dry run with the signed candidate from P2's `package-app.sh` and `SIGNING_IDENTITY` set:

1. Note `launchctl print gui/$(id -u)/dev.paw-proxy`.
2. Run `sudo "/Applications/Paw Proxy.app/Contents/Helpers/paw-proxy" setup`.
3. Confirm exactly one daemon (`pgrep -fl 'paw-proxy run'`), that it runs from the bundle path, and that `https://<route>.test` works.
4. Run `paw-proxy uninstall` followed by `sudo paw-proxy setup --legacy-launchagent` to return to the old setup if needed.

Record the outcome in Scratchpad.

---

## Phase 4: Distribution

**Branches:** `feat/native-app-release` (this repo) and `feat/paw-proxy-cask` (`../homebrew-tap`) · **Depends on:** P3

### 4.1 `release.json` and Taskfile

- Remove the targets `darwin-amd64` and `darwin-universal` (the Intel drop). The `man-pages.tar.gz` artifact moves to the `linux-amd64` target, so the formula still gets it.
- Replace the `darwin-arm64` artifacts with `{ "name": "Paw-Proxy.app.zip", "kind": "zip" }` (an unversioned name, allowed since release-tools v0.2.0).
  - `build:darwin-arm64` runs `macos/Scripts/package-app.sh "$TMPDIR/Paw Proxy.app"` with `APP_VERSION=$RELEASE_VERSION` and zips with `ditto -c -k --keepParent`.
  - `validate:darwin-arm64` unzips to a temp dir, then runs `validate-bundle.sh` and `codesign --verify --deep --strict`.
- Add `"signing": { "macos": { "targets": ["darwin-arm64"], "notarize": true, "staple": true } }`. Confirm in `../release-tools/docs/contracts.md` whether reltool signs the zip or expects a signed `.app` from `build_task`, and pass `SIGNING_IDENTITY` accordingly. Signing identity and notary profile names stay in local config, never in the repo.
- Replace the single `tap` with:
  ```json
  "taps": [
    { "repo": "alexcatdad/homebrew-tap", "kind": "cask",    "package": "paw-proxy", "targets": ["darwin-arm64"] },
    { "repo": "alexcatdad/homebrew-tap", "kind": "formula", "package": "paw-proxy", "targets": ["linux-amd64", "linux-arm64"] }
  ]
  ```
- Bump `RELTOOL_TAG` if `taps[]` needs a release newer than the pinned v0.2.0 (`6036a85` is the commit that added it, and it is the current pin, so likely no bump is needed).
- Run `task release:check` (read-only) and `task release:prepare` locally. **Do not run `task release` without the owner's explicit go-ahead.**

- Flip `appRequired` to `true` in `internal/setup/daemon_install_darwin.go`, update `TestAppRequiredStaysOffUntilCaskShips` accordingly, and pass `--legacy-launchagent` in `lifecycle-tests.sh` and the CI macOS job. Ship this in the same release that publishes the cask.

### 4.x Implementation notes

- `build:darwin-arm64` requires `MACOS_SIGNING_IDENTITY` (reltool passes it from the local config), runs `package-app.sh` with it and zips with `ditto -c -k --keepParent`. reltool notarizes and staples. `BUILD_NUMBER` stays 1; versions are unique per release.
- The bundle now carries `Contents/Resources/man/{paw-proxy,up}.1`, and `validate-bundle.sh` checks them, so the cask installs man pages through `manpage`.
- `taps[]`: the cask gets darwin targets by default; the formula is limited to `linux-amd64` and `linux-arm64`. `man-pages.tar.gz` moved to `linux-amd64`.
- `Formula/paw.rb` is the separate dotfiles tool, not paw-proxy, so there is nothing to decide about it.
- The tap updater only rewrites existing `url`/`sha256` slots, so the tap PR seeds `Casks/paw-proxy.rb` with a placeholder slot for `Paw-Proxy.app.zip` at v1.12.3 and makes the formula Linux-only (`depends_on :linux`) with v1.12.3's real Linux assets. **Merge the tap PR right before `task release`**: before that, the cask points at a zip that does not exist yet and macOS formula users would lose upgrades.
- Release order: merge P3, then this PR, add the paw-proxy signing entry to `~/.config/release-tools/config.json`, merge the tap PR, then `task release` (owner's go-ahead only).

### 4.2 Homebrew tap (`../homebrew-tap`)

- New `Casks/paw-proxy.rb`:
  - `depends_on arch: :arm64`, `depends_on macos: ">= :ventura"`
  - `app "Paw Proxy.app"`
  - `binary "#{appdir}/Paw Proxy.app/Contents/Helpers/paw-proxy"` and the same for `up`
  - man pages via `manpage` if they are bundled into `Contents/Resources/man`; otherwise skip on macOS and note it
  - `uninstall quit: "com.alexcatdad.paw-proxy"`
  - `zap trash: ["~/Library/Application Support/paw-proxy", "~/Library/Logs/paw-proxy.log"]`
  - caveats: run `sudo paw-proxy setup` after install, `paw-proxy restart` after upgrade, and `sudo paw-proxy uninstall` before uninstalling the cask
- `Formula/paw-proxy.rb`: Linux only. On macOS, `odie` with "paw-proxy for macOS is now a cask: brew uninstall paw-proxy && brew install --cask alexcatdad/tap/paw-proxy".
- `Formula/paw.rb`: inventory what it maps to and decide whether to alias, keep or remove it (an open owner decision).
- Follow the tap repo's own instructions (`../homebrew-tap/CLAUDE.md`) and the `release-tools/tap-update/v1` contract. The tap is updated by `reltool` from a published release, not by hand. Only the cask skeleton and the formula's `odie` guard are hand-authored.

**Verify:** `task release:prepare` produces a notarized and stapled zip (`spctl -a -vv`, `stapler validate`). On a scratch machine or user account: `brew install --cask --force ./Casks/paw-proxy.rb`, then `which paw-proxy up`, then `paw-proxy version`.

---

## Phase 5: Docs and installed acceptance

**Branch:** `docs/native-app` · **Depends on:** P4

- `README.md`:
  - install via the cask; the Apple Silicon and macOS 13+ requirement
  - the menu-bar app section, replacing "Live dashboard"
  - `paw-proxy restart`
  - migration notes for formula users
  - Linux unchanged
- `docs/index.html`: replace the `_paw.test` feature card (around line 536) with the menu-bar route view, and update the install snippet.
- `CHANGELOG.md`: removed `_paw.test`; dropped Intel macOS; formula is Linux-only; added the app, the tray and `restart`.
- `CONTRIBUTING.md`: the macOS app development loop (`task macos:dev`), and the signing/notary prerequisites for releases (names only, no identities).
- `CLAUDE.md`: replace the stale backlog and auto-release text with the `task check` / `task macos:test` commands and a pointer to Scratchpad for project memory.
- **Installed acceptance:** a human runs the checklist in the design spec's Testing section on the real cask install. Record each result (pass/fail/not run) in Scratchpad with `record_finding` or `record_project_state`. A checklist item counts as done only once it has been observed.

---

## Risks

| Risk | Mitigation |
|---|---|
| Bundled agent ignores `Sockets` | P0 decides; the daemon already falls back to direct binding |
| Two proxies during migration | `turnOn` boots out and deletes the legacy agent before `register()`; no rollback, the failure reply names `--legacy-launchagent` |
| Users on the formula upgrade and lose the daemon | The formula `odie` message names the exact commands; the cask's first `setup` migrates the agent |
| Login Items approval stalls `setup` | The `requiresApproval` reply is a pending success with clear instructions; the 60 s timeout is followed by a clear message |
| Dev app clobbers the installed agent | The dev bundle has no LaunchAgents and a `DisabledRegistrar`; the validator enforces both |
| Homebrew replaces the bundle under the running daemon | Version/build skew shows "Restart to finish update"; `paw-proxy restart` |
