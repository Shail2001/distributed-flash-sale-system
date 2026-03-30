#!/bin/bash
# Flash Sale API — E2E Smoke Test
# Usage: ./scripts/smoke_test.sh [ALB_URL]
# Default: localhost:8080

BASE_URL=${1:-"http://localhost:8080"}
PASS=0
FAIL=0

check() {
  local desc=$1
  local expected=$2
  local actual=$3
  if echo "$actual" | grep -q "$expected"; then
    echo "✅ $desc"
    ((PASS++))
  else
    echo "❌ $desc — expected: $expected, got: $actual"
    ((FAIL++))
  fi
}

echo "=== Flash Sale API Smoke Test ==="
echo "Target: $BASE_URL"
echo ""

# Reset
R=$(curl -s -X POST $BASE_URL/reset)
check "POST /reset" "reset" "$R"

# Inventory
R=$(curl -s $BASE_URL/inventory)
check "GET /inventory (remaining=100)" "100" "$R"
check "GET /inventory (sold_out=false)" "false" "$R"

# Health
R=$(curl -s $BASE_URL/health)
check "GET /health" "healthy" "$R"

# Purchase
R=$(curl -s -X POST $BASE_URL/purchase \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"smoke-test-1","quantity":1}')
check "POST /purchase (201 accepted)" "accepted" "$R"
check "POST /purchase (remaining=99)" "99" "$R"

# Bad request
R=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE_URL/purchase \
  -H "Content-Type: application/json" \
  -d '{"quantity":1}')
check "POST /purchase (400 bad request)" "400" "$R"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
