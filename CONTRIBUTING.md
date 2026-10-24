# Contributing to paw-proxy

Thanks for your interest in contributing!

## Development Setup

1. **Clone the repo**
   ```bash
   git clone https://github.com/alexcatdad/paw-proxy.git
   cd paw-proxy
   ```

2. **Install Go 1.22+**
   ```bash
   brew install go
   ```

3. **Build**
   ```bash
   go build -o paw-proxy ./cmd/paw-proxy
   go build -o up ./cmd/up
   ```

4. **Run tests**
   ```bash
   go test -v ./...
   ```

5. **Test locally** (requires macOS)
   ```bash
   sudo ./paw-proxy setup
   ./integration-tests.sh
   ```

## Project Structure

```
internal/
├── api/        # Route registry + unix socket API
├── daemon/     # Main daemon orchestrator
├── dns/        # DNS server for .test domains
├── proxy/      # Reverse proxy with WebSocket
├── setup/      # macOS setup/uninstall
└── ssl/        # CA and certificate generation

cmd/
├── paw-proxy/  # Main CLI
└── up/         # Dev server wrapper
```

## Making Changes

1. **Fork the repo** and create a branch
   ```bash
   git checkout -b my-feature
   ```

2. **Make your changes** with tests

3. **Run the linter**
   ```bash
   golangci-lint run
   ```

4. **Run tests**
   ```bash
   go test -v ./...
   ```

5. **Commit** with a clear message
   ```bash
   git commit -m "feat: add cool feature"
   ```

6. **Push** and open a PR

## Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation
- `test:` - Tests
- `chore:` - Maintenance
- `ci:` - CI/CD changes

## Code Style

- Run `go fmt` before committing
- Follow existing patterns in the codebase
- Add tests for new functionality
- Keep functions focused and small

## Releasing

Releases are local and explicit. CI never builds, signs, tags or publishes a release, and no workflow dispatches one. Maintainers release from a coordinator Mac with a checkout of `release-tools` next to this repository (default `../release-tools`, tag fetched) and a local config at `~/.config/release-tools/config.json` (or `$XDG_CONFIG_HOME/release-tools/config.json`); the config lives outside the checkout and holds machine-specific settings only.

```bash
task release:check    # read-only preflight: clean tree, next version, target placement
task release:prepare  # reserve the version, run checks, build every target (no publication)
task release          # prepare, then draft release, SHA-256 verified upload, publish, tap update
task release:resume   # continue an interrupted release with the same version and artifacts
```

Versions are CalVer (`YYYY.M.N`, tags `vYYYY.M.N`, counter per UTC month) allocated by `release-tools` and passed to builds as `RELEASE_VERSION`; they sort above the earlier `1.x` tags. Run `release` only from a clean checkout of a commit that is pushed to GitHub. SSH build workers are optional (`task release:prepare -- --workers`); every target builds on the coordinator by default. See the release-tools documentation for the local config, recovery and worker details.

## Reporting Issues

- Check existing issues first
- Include macOS version and Go version
- Provide steps to reproduce
- Include relevant logs (`~/Library/Logs/paw-proxy.log`)

## Questions?

Open an issue with the `question` label.
