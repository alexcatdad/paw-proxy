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

1. **CA Trust** - The generated CA is trusted system-wide. Keep `~/Library/Application Support/paw-proxy/ca.key` secure.

2. **Local Only** - The proxy only binds to localhost by design. Never expose to external networks.

3. **Route Isolation** - Routes are isolated per working directory to prevent conflicts.
