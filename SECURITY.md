# Security Policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's
[security advisory form](https://github.com/alexcatdad/paw-proxy/security/advisories/new).
Do not open a public issue for a suspected vulnerability. Include a
description, steps to reproduce and the impact you see. Expect an initial
response within seven days.

## Supported versions

Only the latest release is supported. Versions are CalVer (`vYYYY.MM.DD.N`);
fixes ship as a new release, never as a replacement of a published one.

## What this app does

paw-proxy runs a local HTTPS reverse proxy for development and generates
certificates your Mac trusts, so its trust surface is:

- **Privileged setup.** `sudo paw-proxy setup` creates a local CA, trusts it in
  the login keychain (System keychain as a fallback), writes
  `/etc/resolver/test` and installs the daemon;
  `sudo paw-proxy uninstall` reverses it. Nothing else needs root.
- **Local CA.** A 4096-bit RSA CA. Its key stays at
  `~/Library/Application Support/paw-proxy/ca.key` with 0600 permissions and is
  never logged or sent anywhere. Anyone who can read it can mint certificates
  your Mac trusts, so keep it private.
- **Local first.** The proxy and DNS bind to 127.0.0.1. The control API is a
  unix socket created with 0600 permissions. Never expose them to a network.
  The one way off your Mac is `up --tailnet`: the daemon asks the Tailscale
  CLI to serve that route's port to your tailnet (`tailscale serve`, never
  Funnel), refuses a port that Funnel or another live service uses, and
  removes only the Serve routes it created.
- **Input validation.** Route names follow a DNS-safe pattern, upstreams must
  be localhost or loopback (SSRF guard), directories must be absolute and
  clean, and route notifications carry only validated route names, never
  display text.
- **TLS hardening.** Minimum TLS 1.2, ECDHE with AES-GCM or ChaCha20-Poly1305.
- **Resource limits.** Certificate cache (1000 entries), request bodies (1 MB),
  per-endpoint rate limits on the API, a bounded notification queue,
  connection and WebSocket idle timeouts.
- **Route isolation.** Routes are tied to the directory that registered them.
- **Notifications** are posted by Paw Proxy.app itself, never through
  `osascript` or a shell.
- No telemetry, analytics or tracking.

## Release trust

Releases are built and Developer ID signed locally, notarized by Apple and
stapled. Signing keys and notarization credentials stay in the maintainer's
local Keychain. GitHub releases are immutable and carry a release attestation;
verify one with `gh release verify <tag> -R alexcatdad/paw-proxy`. The Homebrew
cask does not remove quarantine.
