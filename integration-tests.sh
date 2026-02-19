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

# Test 5: HTTPS certificate - SAN contains requested hostname
echo "[Test 5] HTTPS certificate SANs..."
CERT_PEM=$(echo | openssl s_client -connect integration-test.test:443 -servername integration-test.test 2>/dev/null | openssl x509)

# Check that the specific hostname is in Subject Alternative Names
if echo "$CERT_PEM" | openssl x509 -noout -text | grep -q "DNS:integration-test.test"; then
  echo "  ✓ Certificate SAN contains integration-test.test"
else
  echo "  ✗ Certificate SAN does not contain integration-test.test"
  echo "$CERT_PEM" | openssl x509 -noout -text | grep -A1 "Subject Alternative Name"
  exit 1
fi

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

# Test 9: Dashboard is accessible
echo "[Test 9] Dashboard at _paw.test..."
DASH_STATUS=$(curl -sk -o /dev/null -w '%{http_code}' https://_paw.test/)
if [ "$DASH_STATUS" -ne 200 ]; then
  echo "  ✗ Dashboard returned $DASH_STATUS, expected 200"
  exit 1
fi
echo "  ✓ Dashboard accessible at _paw.test"

# Test 10: Dashboard API - routes
echo "[Test 10] Dashboard API routes..."
ROUTES_JSON=$(curl -sk https://_paw.test/api/routes)
if ! echo "$ROUTES_JSON" | grep -q '\['; then
  echo "  ✗ /api/routes did not return JSON array"
  exit 1
fi
echo "  ✓ Dashboard API returns routes"

# Test 11: Dashboard API - stats
echo "[Test 11] Dashboard API stats..."
STATS_JSON=$(curl -sk https://_paw.test/api/stats)
if ! echo "$STATS_JSON" | grep -q '"version"'; then
  echo "  ✗ /api/stats did not return version"
  exit 1
fi
echo "  ✓ Dashboard API returns stats"

echo ""
echo "=== All tests passed! ==="
