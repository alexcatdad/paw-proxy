# Security & Code Quality Review

**Date:** 2026-02-06
**Scope:** Full codebase review of all Go source files (~1,600 LOC production, ~1,900 LOC tests)
**Test status:** All tests pass (`go test -race ./...`) except one false-positive in root environments. `go vet` clean.

---

## Executive Summary

paw-proxy is well-architected for a local development tool. The codebase demonstrates strong security awareness: SSRF prevention at two layers, proper input validation, XSS escaping, restrictive file permissions, and socket TOCTOU mitigation via umask. The issues found are mostly **low severity** given the localhost-only threat model, but several are worth addressing for defense-in-depth.

**Findings:** 5 security issues (0 critical, 1 high, 2 medium, 2 low), 8 code quality issues.

---

## Security Findings

### S1. [HIGH] HTTP redirect server open redirect

**File:** `internal/daemon/daemon.go:298`

```go
target := "https://" + r.Host + r.URL.RequestURI()
http.Redirect(w, r, target, http.StatusPermanentRedirect)
```

The HTTP-to-HTTPS redirect blindly trusts `r.Host`, which is client-controlled. An attacker on the local network (or a malicious site triggering a fetch to port 80) can craft a request with `Host: evil.com` to redirect the browser to `https://evil.com/...`. Since the daemon is bound to loopback, exploitability requires the user to visit a crafted URL, but the redirect is a 308 (cached permanently), which amplifies the impact.

**Recommendation:** Validate that `r.Host` ends with `.test` before redirecting. Return 400 for non-.test hosts.

---

### S2. [MEDIUM] WebSocket upstream connects to arbitrary address (bypasses loopback check)

**File:** `internal/proxy/proxy.go:182`

```go
upstreamConn, err := net.DialTimeout("tcp", upstream, 5*time.Second)
```

The HTTP proxy path uses a custom `DialContext` that enforces loopback-only connections (line 24-35). However, the WebSocket path at line 182 calls `net.DialTimeout` directly without any address validation. The `upstream` value comes from the route registry which was validated at registration time, so this is defense-in-depth rather than a direct exploit, but it means a bug in the registry or a race condition could cause the WebSocket path to connect to non-loopback addresses.

**Recommendation:** Extract the loopback validation from `DialContext` into a reusable function and call it in `handleWebSocket` before dialing.

---

### S3. [MEDIUM] HTTPS server missing `ReadTimeout` and `WriteTimeout`

**File:** `internal/daemon/daemon.go:334-338`

```go
server := &http.Server{
    Handler:     http.HandlerFunc(d.handleRequest),
    TLSConfig:   tlsConfig,
    IdleTimeout: 120 * time.Second,
}
```

The HTTPS server has `IdleTimeout` but no `ReadTimeout` or `WriteTimeout`. A slowloris-style client can hold connections indefinitely by sending data very slowly. Since paw-proxy binds to loopback and is single-user, this is low risk, but it means a buggy dev server making recursive requests could exhaust file descriptors.

The HTTP redirect server (line 296-301) also has no timeouts at all.

**Recommendation:** Add `ReadTimeout: 30 * time.Second` and `WriteTimeout: 60 * time.Second` to both HTTP and HTTPS servers. Note that `WriteTimeout` will break SSE connections (tracked as issue #24), so this should be coordinated with that fix.

---

### S4. [LOW] `sanitizeName` can produce names starting with a digit

**File:** `cmd/up/main.go:229-244`

`sanitizeName` lowercases and strips non-alphanumeric characters, but doesn't ensure the result starts with a letter. A directory named `123-project` produces route name `123-project`, which passes `routeNamePattern` validation (regex allows digit starts). However, DNS labels starting with digits are technically non-compliant with RFC 952 (though widely accepted by resolvers). Not a practical issue for `.test` TLD but worth noting.

---

### S5. [LOW] `up` does not validate the `-n` flag through `sanitizeName`

**File:** `cmd/up/main.go:209-213`

```go
func determineName(explicit string) string {
    if explicit != "" {
        return explicit
    }
```

When the user provides `-n` explicitly, the value is passed directly to `registerRoute` without going through `sanitizeName`. The daemon's API validates the name server-side (via `validateRouteName`), so this won't cause a security issue, but it does mean `up -n "My App"` will fail with a confusing API error rather than being sanitized or rejected early with a helpful message.

**Recommendation:** Run `sanitizeName` on the explicit name too, or validate and give a clear error before contacting the daemon.

---

## Code Quality Findings

### Q1. Test failure in root environments

**File:** `internal/ssl/ca_test.go:118-135`

`TestGenerateCA_PemEncodeErrorOnReadOnlyPath` creates a `0555` directory and expects writes to fail. This test passes for normal users but fails when running as root (root bypasses file permission checks on most filesystems). CI runs integration tests as root.

**Recommendation:** Skip the test when running as root:
```go
if os.Geteuid() == 0 {
    t.Skip("skipping: root bypasses directory permissions")
}
```

---

### Q2. No bound on route registry size

**File:** `internal/api/routes.go:40`

The `Register` method has no limit on the number of routes. The rate limiter (10 req/sec) slows registration but doesn't cap the total. A malfunctioning script could create thousands of routes over time. The cert cache has a 1000-entry LRU cap, but the route registry has none.

**Recommendation:** Add a `maxRoutes` constant (e.g., 100) and return an error when exceeded.

---

### Q3. Inconsistent error handling in `json.NewEncoder(w).Encode()`

**Files:** `internal/api/server.go:148,180,227,233`

Multiple handlers call `json.NewEncoder(w).Encode(...)` without checking the returned error. If the client disconnects mid-response, the error is silently dropped.

**Recommendation:** Log encode errors at debug level for observability.

---

### Q4. `r.Clone(r.Context())` copies the full request including body

**File:** `internal/proxy/proxy.go:75`

`r.Clone()` copies the request body reference. For large request bodies, this is fine (it doesn't duplicate the data), but the original `r.Body` is now shared with `outReq.Body`. In practice, `http.Server` doesn't reuse the request after the handler returns, so this is safe in the current code, but it's fragile. Go's `httputil.ReverseProxy` uses `r.Clone(r.Context())` the same way, so this is idiomatic.

No action needed, but worth documenting.

---

### Q5. `daemon_test.go` has minimal coverage (0% of `Run()`)

**File:** `internal/daemon/daemon_test.go`

The daemon package has only redirect-handler tests and no test for `New()`, `Run()`, or `cleanupRoutine()`. The daemon is the most complex component (orchestrating 5 goroutines) and is untested. Bugs here (like the former ticker leak) are caught only by integration tests or manual testing.

**Recommendation:** Add unit tests for:
- `New()` with missing CA (error path)
- `cleanupRoutine()` with a mock registry
- `createHTTPServer()` and `createHTTPSServer()` return correct bind addresses

---

### Q6. `deregisterRoute` and `heartbeat` ignore HTTP errors

**File:** `cmd/up/main.go:295-298,310-315`

```go
func deregisterRoute(client *http.Client, name string) {
    req, _ := http.NewRequest("DELETE", ...)
    client.Do(req)
}
```

Both `deregisterRoute` and `heartbeat` (on error path) silently drop HTTP response status. If deregistration fails, the route stays alive until heartbeat timeout (30s), which is usually fine. But heartbeat failures are only logged at `warning` level, and if the daemon restarts, heartbeat failures will cascade until the `up` process is also restarted.

**Recommendation:** At minimum, log deregistration failures.

---

### Q7. `cmdLogsTail` never exits on EOF

**File:** `cmd/paw-proxy/main.go:445-455`

```go
for {
    n, err := f.Read(buf)
    if n > 0 {
        os.Stdout.Write(buf[:n])
    }
    if err != nil {
        time.Sleep(200 * time.Millisecond)
        continue
    }
}
```

The tail loop treats ALL errors (not just `io.EOF`) as "wait for more data". If the file is deleted or the filesystem encounters an error, this loop will spin forever at 200ms intervals, consuming CPU. It also doesn't handle SIGINT/SIGTERM for clean exit.

**Recommendation:** Only continue on `io.EOF`; exit on other errors. Add signal handling.

---

### Q8. CI `chmod 777` on socket is overly permissive

**File:** `.github/workflows/ci.yml:89`

```yaml
sudo chmod 777 "$HOME/Library/Application Support/paw-proxy/paw-proxy.sock"
```

The CI makes the socket world-accessible. While this is only in CI and necessary because the daemon runs as root while tests run as the CI user, `0660` with a shared group would be more appropriate. Not a production risk since this is CI-only.

---

## Positive Security Observations

These patterns are well-implemented and worth preserving:

1. **Defense-in-depth SSRF prevention** - Validated at both API layer (`server.go:110`) and proxy layer (`proxy.go:32-34`)
2. **Socket TOCTOU mitigation** - Using `umask(0077)` before `net.Listen` (`server.go:69`) instead of `Listen` then `Chmod`
3. **Proper XSS escaping** - All dynamic HTML content uses `html.EscapeString()` (`errorpage.go`)
4. **Copy-on-read from registry** - `Lookup()` and `List()` return value copies, preventing callers from mutating internal state (`routes.go:76,146`)
5. **Cleanup double-check under write lock** - `Cleanup()` re-validates expiry after upgrading from read lock to write lock (`routes.go:139`)
6. **MaxPathLen=0 on CA** - Prevents the local CA from signing intermediate CAs (`ca.go:44`)
7. **Request body size limit** - `MaxBytesReader` prevents memory exhaustion (`server.go:153`)
8. **WebSocket RFC validation** - Checks `Sec-WebSocket-Key` and version before accepting upgrade (`proxy.go:162`)
9. **Idle timeout on WebSocket** - Uses per-read/write deadline reset instead of absolute timeout (`proxy.go:141-158`)
10. **Rate limiting** - Per-endpoint rate limiters on all API endpoints (`server.go:44-48`)
11. **Strong cryptography** - RSA-4096 CA, ECDSA P-256 leaf certs, TLS 1.2 minimum with AEAD cipher suites
12. **Structured logging** - JSON logger with request metadata for audit trail (`daemon.go:88`)

---

## Summary Table

| ID | Severity | Category | File | Issue |
|----|----------|----------|------|-------|
| S1 | HIGH | Security | daemon.go:298 | Open redirect via Host header |
| S2 | MEDIUM | Security | proxy.go:182 | WebSocket dial bypasses loopback check |
| S3 | MEDIUM | Security | daemon.go:334 | Missing Read/WriteTimeout on servers |
| S4 | LOW | Security | cmd/up/main.go:229 | sanitizeName allows digit-leading names |
| S5 | LOW | Security | cmd/up/main.go:209 | -n flag not sanitized client-side |
| Q1 | - | Quality | ca_test.go:118 | Test fails when run as root |
| Q2 | - | Quality | routes.go:40 | No max routes limit |
| Q3 | - | Quality | server.go:148 | JSON encode errors ignored |
| Q4 | - | Quality | proxy.go:75 | r.Clone body sharing (idiomatic, no fix needed) |
| Q5 | - | Quality | daemon_test.go | 0% coverage on daemon core |
| Q6 | - | Quality | cmd/up/main.go:295 | Silent failure on deregistration |
| Q7 | - | Quality | cmd/paw-proxy/main.go:445 | Tail loop doesn't handle non-EOF errors |
| Q8 | - | Quality | ci.yml:89 | chmod 777 on socket in CI |
