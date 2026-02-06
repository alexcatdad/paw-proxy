#!/bin/bash
# Run this locally with `gh` CLI authenticated to create all issues.
# Usage: bash create-issues.sh

set -e

echo "Creating security and code quality issues..."

# S1 - HIGH
gh issue create \
  --title "fix: HTTP redirect server allows open redirect via Host header" \
  --label "bug,security,P0" \
  --body "$(cat <<'EOF'
## Description

The HTTP-to-HTTPS redirect handler in `internal/daemon/daemon.go:298` blindly trusts `r.Host`:

```go
target := "https://" + r.Host + r.URL.RequestURI()
http.Redirect(w, r, target, http.StatusPermanentRedirect)
```

A client can set `Host: evil.com` to get redirected to `https://evil.com/...`. Since this is a 308 (Permanent Redirect), browsers cache it, amplifying the impact.

## Impact

An attacker who can get a user to visit `http://127.0.0.1/path` with a crafted Host header can redirect them to an arbitrary HTTPS URL. Low exploitability (requires local access or CSRF-like scenario) but high impact due to permanent caching.

## Fix

Validate that `r.Host` ends with `.test` before redirecting. Return 400 for non-`.test` hosts:

```go
Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    host := r.Host
    // Strip port if present
    if h, _, err := net.SplitHostPort(host); err == nil {
        host = h
    }
    if !strings.HasSuffix(host, ".test") {
        http.Error(w, "invalid host", http.StatusBadRequest)
        return
    }
    target := "https://" + r.Host + r.URL.RequestURI()
    http.Redirect(w, r, target, http.StatusPermanentRedirect)
}),
```

## Found in

Security & code quality review (2026-02-06), finding S1.
EOF
)"

echo "  Created: S1 - Open redirect"

# S2 - MEDIUM
gh issue create \
  --title "fix: WebSocket dial bypasses loopback-only validation" \
  --label "bug,security,P1" \
  --body "$(cat <<'EOF'
## Description

The HTTP proxy path uses a custom `DialContext` that enforces loopback-only connections (`internal/proxy/proxy.go:24-35`). However, the WebSocket path at line 182 calls `net.DialTimeout("tcp", upstream, 5*time.Second)` directly without any address validation.

```go
// HTTP path (protected):
DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
    // SECURITY: Defense-in-depth — reject non-loopback addresses
    ...
}

// WebSocket path (unprotected):
upstreamConn, err := net.DialTimeout("tcp", upstream, 5*time.Second)
```

The `upstream` value comes from the route registry which was validated at registration time, so this is defense-in-depth rather than a direct exploit. But a bug in the registry or a race condition could cause the WebSocket path to connect to non-loopback addresses.

## Fix

Extract the loopback validation into a shared function and call it in `handleWebSocket` before dialing:

```go
func validateLoopback(addr string) error {
    host, _, err := net.SplitHostPort(addr)
    if err != nil {
        return fmt.Errorf("proxy: split host/port: %w", err)
    }
    ip := net.ParseIP(host)
    if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
        return fmt.Errorf("proxy: refusing connection to non-local host %s", host)
    }
    return nil
}
```

## Found in

Security & code quality review (2026-02-06), finding S2.
EOF
)"

echo "  Created: S2 - WebSocket loopback bypass"

# S3 - MEDIUM
gh issue create \
  --title "fix: HTTPS/HTTP servers missing ReadTimeout and WriteTimeout" \
  --label "bug,security,P1" \
  --body "$(cat <<'EOF'
## Description

The HTTPS server (`internal/daemon/daemon.go:334-338`) has `IdleTimeout` but no `ReadTimeout` or `WriteTimeout`:

```go
server := &http.Server{
    Handler:     http.HandlerFunc(d.handleRequest),
    TLSConfig:   tlsConfig,
    IdleTimeout: 120 * time.Second,
    // Missing: ReadTimeout, WriteTimeout
}
```

The HTTP redirect server (line 296-301) has no timeouts at all.

Without these, a slowloris-style client can hold connections indefinitely by sending data very slowly, eventually exhausting file descriptors.

## Considerations

- `WriteTimeout` will break SSE/streaming connections (see #24)
- May need `ReadHeaderTimeout` separately from `ReadTimeout` to allow large request bodies
- Consider using `http.Server.ReadHeaderTimeout` for the initial header read, and per-handler timeouts for bodies

## Suggested fix

```go
server := &http.Server{
    Handler:           http.HandlerFunc(d.handleRequest),
    TLSConfig:         tlsConfig,
    IdleTimeout:       120 * time.Second,
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       30 * time.Second,
}
```

For the HTTP redirect server, all three timeouts can be short since it only sends a redirect response.

## Found in

Security & code quality review (2026-02-06), finding S3.
EOF
)"

echo "  Created: S3 - Missing server timeouts"

# S5 - LOW
gh issue create \
  --title "fix: up -n flag not sanitized before sending to daemon API" \
  --label "bug,P2" \
  --body "$(cat <<'EOF'
## Description

In `cmd/up/main.go:209-213`, when the user provides an explicit name via `-n`, the value bypasses `sanitizeName`:

```go
func determineName(explicit string) string {
    if explicit != "" {
        return explicit  // Not sanitized!
    }
    // ... package.json and directory name go through sanitizeName
}
```

The daemon validates the name server-side, so this isn't a security issue. But it means `up -n "My App"` fails with a confusing API error like `"invalid route name"` instead of being sanitized or rejected early with a helpful message.

## Fix

Either run `sanitizeName` on the explicit name:
```go
if explicit != "" {
    return sanitizeName(explicit)
}
```

Or validate and give a clear error:
```go
if explicit != "" {
    if !routeNamePattern.MatchString(explicit) {
        fmt.Fprintf(os.Stderr, "Invalid name %q: use only a-z, 0-9, dash, underscore\n", explicit)
        os.Exit(1)
    }
    return explicit
}
```

## Found in

Security & code quality review (2026-02-06), finding S5.
EOF
)"

echo "  Created: S5 - up -n flag not sanitized"

# Q1 - Test failure in root
gh issue create \
  --title "fix: ca_test fails when run as root (PemEncodeErrorOnReadOnlyPath)" \
  --label "bug,P2" \
  --body "$(cat <<'EOF'
## Description

`TestGenerateCA_PemEncodeErrorOnReadOnlyPath` in `internal/ssl/ca_test.go:118` creates a `0555` directory and expects writes to fail. This works for normal users but fails when running as root, since root bypasses file permission checks on most filesystems.

CI integration tests run as root via `sudo`, which can cause this test to fail.

## Fix

```go
func TestGenerateCA_PemEncodeErrorOnReadOnlyPath(t *testing.T) {
    if os.Geteuid() == 0 {
        t.Skip("skipping: root bypasses directory permissions")
    }
    // ... rest of test
}
```

## Found in

Security & code quality review (2026-02-06), finding Q1.
EOF
)"

echo "  Created: Q1 - Test failure as root"

# Q2 - Route registry unbounded
gh issue create \
  --title "fix: route registry has no maximum size limit" \
  --label "enhancement,P2" \
  --body "$(cat <<'EOF'
## Description

The `RouteRegistry.Register()` method in `internal/api/routes.go:40` has no limit on the number of routes. The rate limiter (10 req/sec) slows registration but does not cap the total count. A malfunctioning script could create thousands of routes over time, consuming memory.

The cert cache has a 1000-entry LRU cap (`ssl/cert.go:20`), but the route registry has no equivalent.

## Fix

Add a `maxRoutes` constant and check in `Register()`:

```go
const maxRoutes = 100

func (r *RouteRegistry) Register(name, upstream, dir string) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if len(r.routes) >= maxRoutes {
        return fmt.Errorf("maximum number of routes (%d) reached", maxRoutes)
    }
    // ... existing logic
}
```

## Found in

Security & code quality review (2026-02-06), finding Q2.
EOF
)"

echo "  Created: Q2 - Unbounded route registry"

# Q7 - cmdLogsTail error handling
gh issue create \
  --title "fix: cmdLogsTail spins on non-EOF errors" \
  --label "bug,P2" \
  --body "$(cat <<'EOF'
## Description

In `cmd/paw-proxy/main.go:445-455`, the log tail loop treats ALL read errors as "wait for more data":

```go
for {
    n, err := f.Read(buf)
    if n > 0 {
        os.Stdout.Write(buf[:n])
    }
    if err != nil {
        time.Sleep(200 * time.Millisecond)
        continue  // Treats ALL errors as EOF
    }
}
```

If the log file is deleted, renamed, or the filesystem encounters an error, this loop spins at 200ms intervals indefinitely. It also has no signal handling for clean exit.

## Fix

Only continue on `io.EOF`; exit on other errors. Add signal handling:

```go
for {
    n, err := f.Read(buf)
    if n > 0 {
        os.Stdout.Write(buf[:n])
    }
    if err != nil {
        if err == io.EOF {
            time.Sleep(200 * time.Millisecond)
            continue
        }
        fmt.Fprintf(os.Stderr, "Error reading log: %v\n", err)
        return
    }
}
```

## Found in

Security & code quality review (2026-02-06), finding Q7.
EOF
)"

echo "  Created: Q7 - cmdLogsTail error handling"

echo ""
echo "Done! Created 7 issues."
echo "(S4 omitted — cosmetic RFC 952 concern, not actionable)"
echo "(Q3-Q6, Q8 omitted — minor quality items, can be bundled into cleanup PRs)"
