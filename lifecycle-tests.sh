#!/bin/bash
set -euo pipefail

# lifecycle-tests.sh — Tests setup and uninstall lifecycle
#
# Usage:
#   sudo ./lifecycle-tests.sh [path-to-binary]
#
# Requires: macOS, sudo (for resolver + keychain)
# CI-aware: skips launchd assertions when no GUI session is available.

BINARY="${1:-./paw-proxy}"
SUPPORT_DIR="$HOME/Library/Application Support/paw-proxy"
PLIST_PATH="$HOME/Library/LaunchAgents/dev.paw-proxy.plist"
RESOLVER_PATH="/etc/resolver/test"
SOCKET_PATH="$SUPPORT_DIR/paw-proxy.sock"
REAL_UID=$(id -u "${SUDO_USER:-$(whoami)}")

PASSED=0
FAILED=0
SKIPPED=0

pass() { echo "  ✓ $1"; PASSED=$((PASSED + 1)); }
fail() { echo "  ✗ $1"; FAILED=$((FAILED + 1)); }
skip() { echo "  ⊘ $1 (skipped)"; SKIPPED=$((SKIPPED + 1)); }

# Detect CI — no GUI session means launchd user domain is unavailable
has_gui_session() {
  launchctl print "gui/$REAL_UID" &>/dev/null
}

echo "=== paw-proxy Lifecycle Tests ==="
echo ""

# ─────────────────────────────────────────────────
# Phase 1: Clean slate
# ─────────────────────────────────────────────────
echo "[Phase 1] Ensuring clean slate..."

# Uninstall any existing installation quietly
"$BINARY" uninstall --brew &>/dev/null || true
# Also bootout in case service is loaded
launchctl bootout "gui/$REAL_UID/dev.paw-proxy" &>/dev/null || true

if [ ! -f "$PLIST_PATH" ] && [ ! -f "$RESOLVER_PATH" ] && [ ! -d "$SUPPORT_DIR" ]; then
  pass "Clean slate confirmed"
else
  # Force clean
  rm -f "$PLIST_PATH"
  rm -f "$RESOLVER_PATH"
  rm -rf "$SUPPORT_DIR"
  pass "Clean slate forced"
fi

# ─────────────────────────────────────────────────
# Phase 2: Setup
# ─────────────────────────────────────────────────
echo ""
echo "[Phase 2] Testing setup..."

"$BINARY" setup
SETUP_EXIT=$?

if [ "$SETUP_EXIT" -eq 0 ]; then
  pass "Setup exited cleanly"
else
  fail "Setup exited with status $SETUP_EXIT"
fi

# 2a: Support directory
echo ""
echo "[2a] Support directory..."
if [ -d "$SUPPORT_DIR" ]; then
  pass "Support directory exists"
else
  fail "Support directory missing: $SUPPORT_DIR"
fi

# 2b: CA certificate
echo ""
echo "[2b] CA certificate..."
if [ -f "$SUPPORT_DIR/ca.crt" ] && [ -f "$SUPPORT_DIR/ca.key" ]; then
  pass "CA cert and key exist"
else
  fail "CA cert or key missing"
fi

if openssl x509 -in "$SUPPORT_DIR/ca.crt" -noout -subject 2>/dev/null | grep -qi "paw-proxy"; then
  pass "CA certificate is valid x509 with paw-proxy subject"
else
  fail "CA certificate invalid or wrong subject"
fi

# 2c: Keychain trust
echo ""
echo "[2c] Keychain trust..."
KEYCHAIN_PATH="$HOME/Library/Keychains/login.keychain-db"
if security find-certificate -c "paw-proxy CA" "$KEYCHAIN_PATH" &>/dev/null; then
  pass "CA trusted in login keychain"
elif security find-certificate -c "paw-proxy CA" /Library/Keychains/System.keychain &>/dev/null; then
  pass "CA trusted in System keychain (CI fallback)"
else
  fail "CA not found in any keychain"
fi

# 2d: DNS resolver
echo ""
echo "[2d] DNS resolver..."
if [ -f "$RESOLVER_PATH" ]; then
  pass "Resolver file exists"
else
  fail "Resolver file missing: $RESOLVER_PATH"
fi

if grep -q "nameserver 127.0.0.1" "$RESOLVER_PATH" && grep -q "port 9353" "$RESOLVER_PATH"; then
  pass "Resolver content correct (127.0.0.1:9353)"
else
  fail "Resolver content incorrect"
fi

# 2e: LaunchAgent
echo ""
echo "[2e] LaunchAgent..."
if [ -f "$PLIST_PATH" ]; then
  pass "Plist file installed"
else
  fail "Plist file missing: $PLIST_PATH"
fi

if has_gui_session; then
  if launchctl print "gui/$REAL_UID/dev.paw-proxy" &>/dev/null; then
    pass "LaunchAgent loaded in user domain"
  else
    fail "LaunchAgent not loaded"
  fi
else
  skip "LaunchAgent load check (no GUI session)"
fi

# 2f: Daemon reachable
echo ""
echo "[2f] Daemon health..."
if has_gui_session; then
  # Give daemon a moment to start via launchd
  for i in 1 2 3 4 5; do
    if [ -S "$SOCKET_PATH" ]; then
      break
    fi
    sleep 1
  done

  if curl -sf --unix-socket "$SOCKET_PATH" http://unix/health | grep -q "ok"; then
    pass "Daemon is healthy via launchd"
  else
    fail "Daemon not reachable via socket"
  fi
else
  skip "Daemon health check (no GUI session — daemon not started via launchd)"
fi

# ─────────────────────────────────────────────────
# Phase 3: Idempotent setup (run again)
# ─────────────────────────────────────────────────
echo ""
echo "[Phase 3] Testing idempotent setup (running setup again)..."

"$BINARY" setup
SETUP2_EXIT=$?

if [ "$SETUP2_EXIT" -eq 0 ]; then
  pass "Second setup exited cleanly (idempotent)"
else
  fail "Second setup failed with status $SETUP2_EXIT"
fi

# Verify everything still works after second setup
if [ -f "$SUPPORT_DIR/ca.crt" ] && [ -f "$RESOLVER_PATH" ] && [ -f "$PLIST_PATH" ]; then
  pass "All artifacts intact after second setup"
else
  fail "Artifacts missing after second setup"
fi

# ─────────────────────────────────────────────────
# Phase 4: Uninstall
# ─────────────────────────────────────────────────
echo ""
echo "[Phase 4] Testing uninstall..."

# Stop daemon before uninstall if running
if has_gui_session && [ -S "$SOCKET_PATH" ]; then
  launchctl bootout "gui/$REAL_UID/dev.paw-proxy" &>/dev/null || true
  sleep 1
fi

# Use --brew to skip interactive prompt and auto-remove CA
"$BINARY" uninstall --brew
UNINSTALL_EXIT=$?

if [ "$UNINSTALL_EXIT" -eq 0 ]; then
  pass "Uninstall exited cleanly"
else
  fail "Uninstall exited with status $UNINSTALL_EXIT"
fi

# 4a: LaunchAgent removed
echo ""
echo "[4a] LaunchAgent cleanup..."
if [ ! -f "$PLIST_PATH" ]; then
  pass "Plist file removed"
else
  fail "Plist file still exists: $PLIST_PATH"
fi

if has_gui_session; then
  if ! launchctl print "gui/$REAL_UID/dev.paw-proxy" &>/dev/null; then
    pass "LaunchAgent unloaded from user domain"
  else
    fail "LaunchAgent still loaded"
  fi
else
  skip "LaunchAgent unload check (no GUI session)"
fi

# 4b: DNS resolver removed
echo ""
echo "[4b] DNS resolver cleanup..."
if [ ! -f "$RESOLVER_PATH" ]; then
  pass "Resolver file removed"
else
  fail "Resolver file still exists: $RESOLVER_PATH"
fi

# 4c: CA removed from keychain
echo ""
echo "[4c] Keychain cleanup..."
if ! security find-certificate -c "paw-proxy CA" "$KEYCHAIN_PATH" &>/dev/null; then
  pass "CA removed from login keychain"
else
  fail "CA still in login keychain"
fi

# 4d: Support directory removed
echo ""
echo "[4d] Support directory cleanup..."
if [ ! -d "$SUPPORT_DIR" ]; then
  pass "Support directory removed"
else
  fail "Support directory still exists: $SUPPORT_DIR"
fi

# ─────────────────────────────────────────────────
# Phase 5: Idempotent uninstall (run again on clean system)
# ─────────────────────────────────────────────────
echo ""
echo "[Phase 5] Testing idempotent uninstall (running uninstall on clean system)..."

"$BINARY" uninstall --brew
UNINSTALL2_EXIT=$?

if [ "$UNINSTALL2_EXIT" -eq 0 ]; then
  pass "Second uninstall exited cleanly (idempotent)"
else
  fail "Second uninstall failed with status $UNINSTALL2_EXIT"
fi

# ─────────────────────────────────────────────────
# Phase 6: Reinstall (setup after uninstall)
# ─────────────────────────────────────────────────
echo ""
echo "[Phase 6] Testing reinstall (setup after uninstall)..."

"$BINARY" setup
REINSTALL_EXIT=$?

if [ "$REINSTALL_EXIT" -eq 0 ]; then
  pass "Reinstall exited cleanly"
else
  fail "Reinstall failed with status $REINSTALL_EXIT"
fi

if [ -f "$SUPPORT_DIR/ca.crt" ] && [ -f "$RESOLVER_PATH" ] && [ -f "$PLIST_PATH" ]; then
  pass "All artifacts restored after reinstall"
else
  fail "Artifacts missing after reinstall"
fi

# Clean up after ourselves
"$BINARY" uninstall --brew &>/dev/null || true

# ─────────────────────────────────────────────────
# Results
# ─────────────────────────────────────────────────
echo ""
echo "==========================================="
echo "Results: $PASSED passed, $FAILED failed, $SKIPPED skipped"
echo "==========================================="

if [ "$FAILED" -gt 0 ]; then
  exit 1
fi
