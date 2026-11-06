#!/bin/sh
# Swift coverage gate: runs the menu-bar app's tests (warnings are errors,
# live socket test included) with coverage and fails when line coverage of
# the PawProxyNative library is below the floor. The floor is the measured
# coverage rounded down, minus 2; raise it when coverage rises.
set -eu

floor=${SWIFT_COVERAGE_FLOOR:-84}

cd "$(dirname "$0")/../macos"

PAWPROXY_TEST_DAEMON=1 swift test --enable-code-coverage -Xswiftc -warnings-as-errors

bin=$(swift build --show-bin-path)
profdata="$(dirname "$(swift test --show-codecov-path)")/default.profdata"
xctest=$(find "$bin" -maxdepth 1 -name '*Tests.xctest' | head -n 1)
if [ -z "$xctest" ] || [ ! -f "$profdata" ]; then
	printf '%s\n' 'Could not find the test bundle or coverage profile.' >&2
	exit 1
fi
name=$(basename "$xctest" .xctest)

mkdir -p ../coverage
xcrun llvm-cov report "$xctest/Contents/MacOS/$name" -instr-profile="$profdata" \
	"$PWD/Sources/PawProxyNative" > ../coverage/swift-summary.txt

# TOTAL row: regions, missed, cover, functions, missed, executed, lines, missed, cover
total=$(awk '$1 == "TOTAL" { gsub(/%/, "", $10); print $10 }' ../coverage/swift-summary.txt)
if [ -z "$total" ]; then
	printf '%s\n' 'Could not read total Swift coverage.' >&2
	exit 1
fi
if ! awk -v total="$total" -v floor="$floor" 'BEGIN { exit !(total >= floor) }'; then
	printf 'PawProxyNative line coverage %s%% is below the %s%% floor.\n' "$total" "$floor" >&2
	exit 1
fi
printf 'PawProxyNative line coverage: %s%% (%s%% floor)\n' "$total" "$floor"
