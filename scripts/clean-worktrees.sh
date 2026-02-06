#!/usr/bin/env bash
set -euo pipefail

# Clean up git worktrees whose branches have been deleted from remote
# Usage: ./scripts/clean-worktrees.sh

echo "Fetching and pruning remote refs..."
git fetch --prune

cleaned=0
repo_root=$(git rev-parse --show-toplevel)

while IFS= read -r wt; do
    # Skip main worktree
    if [ "$wt" = "$repo_root" ]; then
        continue
    fi

    branch=$(git -C "$wt" branch --show-current 2>/dev/null || true)
    if [ -z "$branch" ]; then
        continue
    fi

    # Check if the branch still exists on remote
    if ! git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
        echo "Removing worktree: $wt (branch: $branch)"
        git worktree remove --force "$wt"
        git branch -D "$branch" 2>/dev/null || true
        cleaned=$((cleaned + 1))
    fi
done < <(git worktree list --porcelain | sed -n 's/^worktree //p')

echo "Done. Cleaned $cleaned worktree(s)."
