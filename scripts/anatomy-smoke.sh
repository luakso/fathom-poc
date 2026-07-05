#!/usr/bin/env bash
# Anatomy v2 smoke: API endpoints + SPA serving against a running server.
# Usage: scripts/anatomy-smoke.sh [base_url]   (default http://localhost:8090)
set -euo pipefail
BASE="${1:-http://localhost:8090}"

fail() { echo "FAIL: $1" >&2; exit 1; }
get() { curl -fsS --max-time 10 "$BASE$1"; }

echo "== meta"
get /api/meta | grep -q dataMaxDay || fail "meta missing dataMaxDay"

ADDR=$(get "/api/base/leaderboard?role=payee&window=all&sort=volume&limit=1" | sed -n 's/.*"address":"\(0x[0-9a-f]*\)".*/\1/p' | head -1)
[ -n "$ADDR" ] || fail "leaderboard returned no rows (run anatomy rollup?)"
echo "== top payee: $ADDR"

for ep in "" "/neighbors" "/timeline?lens=known" "/fingerprint?lens=known" \
  "/counterparties?role=payee&lens=known"; do
  echo "== entity$ep"
  get "/api/base/entity/$ADDR$ep" >/dev/null || fail "entity$ep"
done

# Payments is the only live query against the payments table; mega-volume
# subjects (top-1 by all-time volume is a 1.6M-txn outlier) can hit the
# documented v1 timeout in lists.go (composite indexes deferred for disk
# headroom). Probe it with a lower-rank payee so the smoke stays deterministic.
ADDR2=$(get "/api/base/leaderboard?role=payee&window=all&sort=volume&limit=25" | sed -n 's/"address":"\(0x[0-9a-f]*\)"/\1\n/gp' | grep -o '0x[0-9a-f]\{40\}' | tail -1)
[ -n "$ADDR2" ] || fail "could not pick a mid-rank payee"
echo "== payments payee: $ADDR2"
get "/api/base/entity/$ADDR2/payments?role=payee&lens=known&limit=5" >/dev/null || fail "payments"

TX=$(get "/api/base/entity/$ADDR2/payments?role=payee&lens=known&limit=1" | sed -n 's/.*"txHash":"\(0x[0-9a-f]*\)".*/\1/p' | head -1)
[ -n "$TX" ] && { echo "== tx $TX"; get "/api/base/tx/$TX" | grep -q '"nodes"' || fail "tx dossier"; }

echo "== SPA"
get / | grep -qi '<div id="root">' || fail "index.html not served"
get "/base/address/$ADDR" | grep -qi '<div id="root">' || fail "SPA fallback broken"
get /api/base/entity/zzz >/dev/null 2>&1 && fail "malformed address should 400"

echo "OK: anatomy v2 smoke passed"
