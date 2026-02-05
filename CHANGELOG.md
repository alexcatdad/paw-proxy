# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
