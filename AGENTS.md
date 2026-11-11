# paw-proxy

## Purpose

Zero-config HTTPS for local development: `https://<name>.test` for any dev server, with a trusted local CA, `.test` DNS and a reverse proxy. macOS menu-bar app (Swift 6, macOS 13+, Apple Silicon), with a Go backend: the `paw-proxy` daemon and setup CLI, and the `up` wrapper that registers routes and runs dev servers.

## Invariants

- Local first: the proxy, DNS and API listen on loopback or a 0600 unix socket. The only way off the Mac is `up --tailnet`: the daemon asks the Tailscale CLI to serve that route's port to the tailnet, never Funnel, falls back to `.test` on any failure, and removes only Serve routes it created. Upstreams are restricted to localhost (SSRF guard); route names, directories and request bodies are validated and size-limited.
- The CA private key stays at 0600 in `~/Library/Application Support/paw-proxy/`; it is never logged, exported or sent anywhere. Setup and uninstall are the only privileged (`sudo`) operations.
- Secrets live only in the macOS Keychain. No plaintext fallback, no secrets in logs.
- Notifications are posted by the app bundle via MenubarKit: opt-in, permission requested from the user toggle, silent unless the user enables sound. Never `osascript`, `terminal-notifier` or a bare binary. `up` only reports route events to the daemon (`POST /notifications`); the app reads them with `GET /notifications?after=<cursor>`, which also returns a per-boot `epoch`; the cursor is only valid within one epoch.
- The tray uses MenubarKit's 18pt template icon, status badge and status-aware accessibility label.
- No third-party dependencies in the shipped app without a recorded decision. First-party `alexcatdad/menubar-kit` is allowed, pinned to the exact commit of a release tag (SwiftPM cannot resolve four-part CalVer tags), with the tag named in a comment. The Go backend's only direct dependency is `github.com/miekg/dns`.
- No telemetry, analytics or update checks over the network.

## Layout

- `macos/` — Swift package: `PawProxy` app target (UI glue), `PawProxyNative` library, tests. `macos/Scripts/` packages and validates `Paw Proxy.app`.
- `cmd/paw-proxy`, `cmd/up` — the two Go binaries. `internal/` — API and route registry (`api`), daemon, DNS, proxy, TLS (`ssl`), `tailnet`, setup, paths.
- `docs/` — GitHub Pages site (`docs/index.html`) and design notes (`docs/superpowers/`). User-facing product truth lives in `README.md` and `man/`.

## Build & verify

- `task check` — the full local gate: gofmt, go vet, golangci-lint, swift-format (strict), Go and Swift builds (Swift warnings are errors), race tests, Go and Swift coverage floors (`task coverage`), package-script tests, govulncheck. CI runs the same tasks.
- `task lint`, `task build`, `task test`, `task security` — what CI calls (`alexcatdad/ci-actions` validate-pr/validate-main).
- `integration-tests.sh` and `lifecycle-tests.sh` change system state (`sudo paw-proxy setup`); run them by hand on a machine you can reset.
- When shipped behaviour changes, update `README.md`, `docs/index.html` and the man pages, and add tests.
- Toolchain: Go 1.27.4, Swift 6 on Xcode for macOS 26, Task.

## Release

- Local only, with release-tools (`task release:check`, `task release:prepare`, `task release`). Developer ID signed, notarized and stapled; keys stay in the local Keychain.
- Versions are CalVer `vYYYY.MM.DD.N`. Releases are immutable: never replace published assets or move tags.
- Distribution: Homebrew cask in `alexcatdad/homebrew-tap`. Never strip quarantine.

## Git

- Canonical host: GitHub `alexcatdad/paw-proxy`.
- Branch from `main`, PR back to `main`, squash-merge. Conventional commit messages; reference the issue (`closes #N`) and read it first (`gh issue view N`).
- Required checks: `pr / leaks`, `pr / lint`, `pr / build`, `pr / deps`, and the code-scanning rule (`codeql / <lang>` checks, high and above). `pr / test` (`task test`) also runs on every PR. Copilot reviews every PR.
- Actions are pinned to full commit SHAs; workflows start from `permissions: {}`.
- Do not wait on or poll CI; one status check when a decision depends on it.

## Memory

Decisions, constraints and project state live in Scratchpad (project `paw-proxy`), not in repo files. Read context there before substantial work and record meaningful decisions there.
