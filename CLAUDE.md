# CLAUDE.md

## Project Overview

paw-proxy is a zero-config HTTPS proxy for local macOS development. It provides two binaries:
- `paw-proxy` - Daemon and setup/management CLI
- `up` - Command wrapper that registers routes and runs dev servers

### Build & Verify
- `go test -v -race ./...` — run all tests with race detector
- `go vet ./...` — static analysis
- `go build ./cmd/paw-proxy && go build ./cmd/up` — build both binaries

### When Adding Features
- Update `README.md` (features list, usage section, command reference)
- Update `docs/index.html` (GitHub Pages — features grid, demo sections)
- Add tests for new functionality

### PR Workflow
1. Create branch: `fix/issue-{N}` or `feat/issue-{N}`
2. Implement the fix (read the GitHub issue: `gh issue view {N}`)
3. Run `go test -v -race ./...` — all tests must pass
4. Run `go vet ./...` — must be clean
5. If you added new functionality, add tests
6. Commit with descriptive message referencing the issue: `fix: bind HTTP/HTTPS to loopback only (closes #40)`
7. Push and create PR with `gh pr create`
8. Enable auto-merge: `gh pr merge <PR_NUMBER> --auto --squash`

### CodeRabbit Review Handling

> **Note:** This workflow requires `gh` CLI (local/dev environments). For Cloud Sessions where `gh` is unavailable, see the Cloud PR Workflow section below.

After creating a PR, follow this loop to resolve CodeRabbit review threads:

1. **Wait 60 seconds** after PR creation for CodeRabbit to post its review.
2. **Check for unresolved threads:**
   ```bash
   gh pr view <PR_NUMBER> --json reviewThreads --jq '.reviewThreads[] | select(.isResolved == false)'
   ```
3. **For each unresolved thread:**
   - Read the suggestion carefully.
   - If it is a **valid code fix** (not just a style nit), apply the change to the codebase.
   - If it is a **non-actionable comment or style nit**, resolve the thread with a brief explanation.
4. **After applying any code fixes**, commit and push the changes.
5. **Wait 30 seconds** for CodeRabbit to re-review the updated code.
6. **Re-check for unresolved threads** (repeat from step 2).
7. **Repeat until no unresolved threads remain**, up to a maximum of **3 iterations** to avoid infinite loops.

**Resolving a thread via GraphQL:**
```bash
gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "THREAD_ID"}) { thread { isResolved } } }'
```

**Key rules:**
- Do not resolve a thread without reading and considering the suggestion first.
- Do not check for threads earlier than 60 seconds after PR creation — CodeRabbit needs time to analyze.
- Always re-check after force-pushing fixes — new reviews may appear on changed code.
- If 3 iterations pass and threads remain, leave a PR comment summarizing what is unresolved and stop.

## Backlog

All issues are independently implementable (no blocking dependencies except #13 which needs #3 first).

Use `gh issue view {N}` to read the full issue description before implementing.
Use `gh issue list --state open` to see all open issues.
Use `gh issue list --label P0` (or P1, P2, P3) to filter by priority.
Use `closes #N` in commit message or PR body to auto-close issues on merge.

### Quick Reference

**Tiny fixes (1-4 lines):** #4, #34, #35, #40, #41, #48, #49, #52
**Small fixes (5-20 lines):** #8, #24, #28, #43, #44, #45, #46, #47, #50, #51
**Medium (20-100 lines):** #6, #7, #12, #22, #25, #26, #32, #36, #37, #42, #53
**Large (100+ lines):** #3, #10, #11, #13, #15, #20, #26, #27, #31, #33, #55

### Combined Issues (dependent work merged)
- **#3** = graceful shutdown + ticker leak + socket cleanup
- **#20** = HTTP/2 + WebSocket over HTTP/2
- **#11** = structured logging + TLS errors + access logs
- **#42** = mutable pointer fix + lock contention
- **#55** = version ldflags + Homebrew tap
- **#13** = launchd socket activation + plist fix (blocked by #3)
