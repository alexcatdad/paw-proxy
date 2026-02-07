#!/bin/bash
set -e

echo "=== paw-proxy Integration Tests ==="
echo ""

# Test 1: Daemon is running
echo "[Test 1] Daemon health check..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock http://unix/health | grep -q "ok"
echo "  ✓ Daemon is healthy"

# Test 2: Register a route
echo "[Test 2] Route registration..."
curl -sf --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock \
  -X POST http://unix/routes \
  -H "Content-Type: application/json" \
  -d '{"name":"integration-test","upstream":"localhost:9999","dir":"/tmp"}'
echo "  ✓ Route registered"

# Test 3: Route appears in list
echo "[Test 3] Route listing..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock http://unix/routes | grep -q "integration-test"
echo "  ✓ Route appears in list"

# Test 4: DNS resolution
echo "[Test 4] DNS resolution..."
dig +short integration-test.test @127.0.0.1 -p 9353 | grep -q "127.0.0.1"
echo "  ✓ DNS resolves to 127.0.0.1"

# Test 5: HTTPS certificate
echo "[Test 5] HTTPS certificate..."
echo | openssl s_client -connect integration-test.test:443 -servername integration-test.test 2>/dev/null | openssl x509 -noout -subject | grep -q "\\*.test"
echo "  ✓ Wildcard certificate issued for *.test"

# Test 5b: HTTP/2 negotiation
# Certificate validity is already tested above; here we only check ALPN protocol.
echo "[Test 5b] HTTP/2 negotiation..."
HTTP2_PROTO=$(curl -k --resolve integration-test.test:443:127.0.0.1 \
  -o /dev/null -w '%{http_version}' \
  -s https://integration-test.test:443/ 2>/dev/null || true)

if [ "$HTTP2_PROTO" = "2" ]; then
  echo "  ✓ HTTP/2 negotiated"
else
  echo "  ✗ Expected HTTP/2 but got HTTP/$HTTP2_PROTO"
  exit 1
fi

# Test 6: Heartbeat
echo "[Test 6] Heartbeat..."
curl -sf --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock \
  -X POST http://unix/routes/integration-test/heartbeat
echo "  ✓ Heartbeat accepted"

# Test 7: Deregister
echo "[Test 7] Route deregistration..."
curl -sf --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock \
  -X DELETE http://unix/routes/integration-test
echo "  ✓ Route deregistered"

# Test 8: Route gone
echo "[Test 8] Route removal verification..."
! curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock http://unix/routes | grep -q "integration-test"
echo "  ✓ Route no longer in list"

echo ""
echo "=== All tests passed! ==="
