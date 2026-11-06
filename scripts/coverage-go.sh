#!/bin/sh
# Go coverage gate: race tests over internal/ with atomic coverage, failing
# below the floor. The floor is the measured coverage rounded down, minus 2;
# raise it when coverage rises, never lower it to make a change pass.
set -eu

floor=${GO_COVERAGE_FLOOR:-65}

cd "$(dirname "$0")/.."
mkdir -p coverage

# Unix socket paths are limited to about 100 bytes, so the API and daemon
# tests need a short temp dir.
short=$(mktemp -d /tmp/pawproxy-cov.XXXXXX)
trap 'rm -rf "$short"' EXIT

# internal/api/testfixture is a main package used only by the Swift tests.
pkgs=$(go list ./internal/... | grep -v /testfixture | paste -sd, -)
TMPDIR="$short" go test -count=1 -race -covermode=atomic -coverpkg="$pkgs" \
	-coverprofile=coverage/go.out ./internal/...
go tool cover -func=coverage/go.out > coverage/go-summary.txt
go tool cover -html=coverage/go.out -o coverage/go.html

total=$(awk '/^total:/ { gsub(/%/, "", $3); print $3 }' coverage/go-summary.txt)
if [ -z "$total" ]; then
	printf '%s\n' 'Could not read total Go coverage.' >&2
	exit 1
fi
if ! awk -v total="$total" -v floor="$floor" 'BEGIN { exit !(total >= floor) }'; then
	printf 'Go internal coverage %s%% is below the %s%% floor.\n' "$total" "$floor" >&2
	exit 1
fi
printf 'Go internal coverage: %s%% (%s%% floor)\n' "$total" "$floor"
