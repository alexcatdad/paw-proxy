# Cloud Prompt Template for paw-proxy Issues

Copy and paste the template below into a Claude Code cloud session, replacing `{N}` with the issue number.

---

## Template

```
Fix GitHub issue #{N} in this repository.

## Instructions

1. Read CLAUDE.md first — it contains the full architecture, conventions, and code locations.
2. Read the issue details: use WebFetch on https://github.com/alexcatdad/paw-proxy/issues/{N}
3. Read the "Cloud Session Notes" section of CLAUDE.md for environment constraints.
4. Implement the fix following the coding conventions in CLAUDE.md.
5. Run `go test -v -race ./...` and `go vet ./...` — both must pass.
6. If the issue adds new behavior, add tests.
7. Commit to a new branch named `fix/issue-{N}` (or `feat/issue-{N}` for features).
8. Push the branch with `git push -u origin <branch-name>`.
9. Do NOT attempt to create a PR — `gh` and GitHub API auth are unavailable in this environment.
10. Output the branch name and a suggested PR title + body (using the format from CLAUDE.md) so I can create the PR manually.
```

---

## Notes

- CLAUDE.md is the single source of truth. It has architecture, code locations with line numbers, coding conventions, and the full backlog.
- Cloud sessions cannot run `golangci-lint` or macOS integration tests. Unit tests and `go vet` are sufficient.
- Cloud sessions cannot create PRs. The session will push the branch and you create the PR locally with:
  ```bash
  gh pr create --head <branch-name> --title "..." --body "..."
  ```
- For tiny issues (1-4 lines), expect the session to finish in under 2 minutes.
- For large issues (100+ lines), consider splitting or providing additional guidance in the prompt.
