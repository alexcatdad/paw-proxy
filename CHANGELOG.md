# Changelog

All notable changes are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are CalVer
`vYYYY.MM.DD.N` (earlier releases used SemVer).

## [Unreleased]

### Changed
- macOS only: Linux release targets and Linux-only code are removed.
- `up` no longer shows notifications through `osascript`. It reports route
  events to the daemon (`POST /notifications`), and Paw Proxy.app reads them
  from `GET /notifications` to post them itself.
- Paw Proxy.app uses `alexcatdad/menubar-kit`: a constant paw tray glyph with
  a badge for any state that needs attention, a status-aware VoiceOver label,
  the shared panel layout and a Settings window.
- CI is validation only, through `alexcatdad/ci-actions`; `task check` runs the
  same lint, build, test and security tasks locally.
- Go 1.27.4; `github.com/miekg/dns` 1.1.73.

### Added
- `up --tailnet` also serves the route on your tailnet through Tailscale Serve
  at `https://<machine>.<tailnet>.ts.net:<port>`; the port is derived from the
  route name, so the URL is stable. The command gets `APP_TAILNET_URL`,
  `paw-proxy status` and the menu bar show the URL, and the daemon removes the
  Serve route when the route ends. Without Tailscale it warns and serves
  `.test` only.
- Paw Proxy.app posts `up`'s "project is live" and "stopped" notifications
  when **Notify me when projects go live** is on (off by default, silent).

## [1.1.0] - 2026-02-05

### Security
- Increased CA key size from 2048 to 4096 bits
- Unix socket permissions restricted to owner-only (0600)
- LaunchAgent now binds to 127.0.0.1 instead of 0.0.0.0
- TLS hardening: minimum TLS 1.2, secure cipher suites only
- Log file permissions restricted to owner-only (0600)
- Added input validation for route names, upstreams (SSRF prevention), and directories
- Certificate cache now has LRU limit (1000 entries) and expiry checking
- WebSocket connections have 1-hour idle timeout
- HTTP server timeouts configured (read: 30s, write: 60s, idle: 120s)
- Request body size limited to 1MB

## [1.0.1] - 2026-02-05

### Added
- Initial public release
- `paw-proxy` daemon with DNS server, SSL generation, and reverse proxy
- `up` command wrapper for dev servers
- Automatic port allocation and environment variable injection
- WebSocket support for hot reload
- macOS LaunchAgent integration
- GitHub Actions CI/CD pipeline

### Fixed
- Cross-platform build compatibility for CI
- Release workflow permissions

## [1.0.0] - 2026-02-05

Initial release (superseded by 1.0.1 due to CI fixes).
