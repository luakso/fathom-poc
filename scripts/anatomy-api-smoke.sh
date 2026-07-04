#!/usr/bin/env bash
# Smoke-checks the anatomy v2 API against the real local DB.
# Usage: just anatomy (in another shell), then: ./scripts/anatomy-api-smoke.sh
set -euo pipefail
BASE="${ANATOMY_URL:-http://localhost:8090}"
PAYER="0x5b2b12361cf46cb78945813f269ce505bc65f2b1"   # tollbit fleet payer
PAYEE="0xa9dd7cc9cbf0e05551332209289f04be36bc2315"   # actual tollbit payee (no label in identity catalog)

check() { # check NAME URL JQ_EXPR EXPECTED
  local name=$1 url=$2 expr=$3 want=$4
  local got
  got=$(curl -sf "$BASE$url" | python3 -c "import json,sys; d=json.load(sys.stdin); print($expr)")
  if [ "$got" = "$want" ]; then echo "ok   $name ($got)"; else echo "FAIL $name: got $got want $want"; exit 1; fi
}

check meta-day        "/api/meta"                                   "d['dataMaxDay']"                       "2026-06-06"
check entity-roles    "/api/base/entity/$PAYER"                     "','.join(d['roles'])"                  "payer"
check entity-txns     "/api/base/entity/$PAYER"                     "d['summaries']['payer']['known']['txnCount']" "20389"
check entity-cps      "/api/base/entity/$PAYER"                     "d['summaries']['payer']['known']['distinctCounterparties']" "1"
check neighbors-top   "/api/base/entity/$PAYER/neighbors"           "d['payees']['rows'][0]['address']"     "$PAYEE"
check neighbors-label "/api/base/entity/$PAYER/neighbors"           "d['payees']['rows'][0].get('label','')" ""
check neighbors-share "/api/base/entity/$PAYER/neighbors"           "d['payees']['rows'][0]['share']"       "1.000000"
check fp-top1         "/api/base/entity/$PAYER/fingerprint"         "d['roles']['payer']['top1Share']"      "1.000000"
check timeline-role   "/api/base/entity/$PAYER/timeline"            "list(d['roles'].keys())[0]"            "payer"
check payments-page   "/api/base/entity/$PAYER/payments?role=payer&limit=5" "len(d['rows'])"               "5"
check leaderboard     "/api/base/leaderboard?role=payee&window=all&sort=volume" "d['rows'][0]['rank']"     "1"
check tx-dossier      "/api/base/tx/0x0e11881999fe9e23eefffc375f86e096026c31f0db648aad7f441eb98b9de05e" "d['chain']" "base"
echo "ALL OK"
