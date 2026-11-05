# Contributing

Thanks for helping. Read [AGENTS.md](AGENTS.md) first: it holds the project's
invariants, layout and checks, for people and agents alike.

## Setup

```sh
brew install go go-task
task check
```

`task check` needs Xcode (Swift 6) for the menu-bar app and network access for
`govulncheck`. To try changes end to end, run `sudo ./paw-proxy setup` on a
machine you can reset, then `./integration-tests.sh`. Logs are in
`~/Library/Logs/paw-proxy.log`.

## Changes

- Branch from `main` and open a pull request back to `main`.
- Use Conventional Commit messages (`feat:`, `fix:`, `docs:`, `test:`, `ci:`, `chore:`).
- Run `task check` before pushing; add tests for new behaviour.
- Update `README.md`, `docs/index.html` and `man/` when shipped behaviour changes.
- Use synthetic fixtures, never real credentials or personal data.
- Describe the observable problem, the change, and the checks you ran.

Required checks and a Copilot review run on every pull request. Pull requests
are squash-merged.

## Releases

Maintainers release locally with release-tools; see the Release section of
AGENTS.md. Contributors never need signing credentials.

```sh
task release:check    # read-only preflight: clean tree, next version, target placement
task release:prepare  # reserve the version, run checks, build every target (no publication)
task release          # prepare, then draft release, SHA-256 verified upload, publish, tap update
task release:resume   # continue an interrupted release with the same version and artifacts
```

Releasing needs a checkout of `release-tools` next to this repository (default
`../release-tools`) and a local config at `~/.config/release-tools/config.json`.
