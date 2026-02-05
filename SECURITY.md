# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | :white_check_mark: |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email or GitHub's private vulnerability reporting.

When reporting, include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fixes

You can expect a response within 48 hours. We'll work with you to understand and address the issue.

## Security Considerations

paw-proxy runs with elevated privileges (ports 80/443) and generates trusted certificates. Key security notes:

1. **CA Trust** - The generated CA (4096-bit RSA) is trusted system-wide. Keep `~/Library/Application Support/paw-proxy/ca.key` secure (0600 permissions).

2. **Local Only** - The proxy binds exclusively to 127.0.0.1. Never expose to external networks.

3. **Route Isolation** - Routes are isolated per working directory to prevent conflicts.

4. **TLS Hardening** - Minimum TLS 1.2, secure cipher suites only (ECDHE with AES-GCM or ChaCha20-Poly1305).

5. **Input Validation** - Route names are alphanumeric only, upstreams restricted to localhost (SSRF prevention).

6. **Resource Limits** - Certificate cache (1000 max), request body size (1MB), connection timeouts configured.
