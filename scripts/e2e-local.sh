#!/usr/bin/env bash
# Manual e2e against a LIVE Sulu backend (make dev / make backend).
# Required env: SULU_URL, SULU_TOKEN, SULU_PROJECT_ID.
set -euo pipefail
: "${SULU_URL:?SULU_URL required (e.g. http://localhost:8080)}"
: "${SULU_TOKEN:?SULU_TOKEN required (Profile → API tokens)}"
: "${SULU_PROJECT_ID:?SULU_PROJECT_ID required}"

cd "$(dirname "$0")/.."
BIN=$(mktemp -d)/suluctl
go build -o "$BIN" .

echo "=== 1. upload: allure-results dir (results + container + attachments) ==="
OUT=$("$BIN" upload --results testdata/e2e/allure-results \
  --launch-name "suluctl e2e allure $(date +%H:%M:%S)" --tag suluctl-e2e | tee /dev/stderr)
echo "$OUT" | grep -Eq "parsed +[1-9]" || { echo "FAIL: no parsed files"; exit 1; }
echo "$OUT" | grep -q "/app/launches/" || { echo "FAIL: no launch link"; exit 1; }

echo "=== 2. upload: single JUnit XML file ==="
"$BIN" upload --results testdata/e2e/TEST-com.example.LoginTest.xml \
  --launch-name "suluctl e2e junit $(date +%H:%M:%S)" \
  | grep -Eq "parsed +1" || { echo "FAIL: junit xml not parsed"; exit 1; }

echo "=== 3. upload: re-upload same dir into a new launch (idempotency / namespacing) ==="
"$BIN" upload --results testdata/e2e/allure-results \
  --launch-name "suluctl e2e re-import $(date +%H:%M:%S)" \
  | grep -Eq "parsed +[1-9]" || { echo "FAIL: re-import must produce a full second launch"; exit 1; }

echo "=== 4. watch: file written mid-run + exit-code passthrough + ingestion assert ==="
DIR=$(mktemp -d)
WOUT=$(mktemp)
set +e
"$BIN" watch --results "$DIR" --launch-name "suluctl e2e watch $(date +%H:%M:%S)" -- \
  sh -c "sleep 1; cp testdata/e2e/TEST-com.example.LoginTest.xml '$DIR/'; sleep 5; exit 5" \
  > "$WOUT" 2>&1
CODE=$?
set -e
cat "$WOUT"
[ "$CODE" -eq 5 ] || { echo "FAIL: expected exit 5, got $CODE"; exit 1; }
grep -Eq "parsed +1" "$WOUT" || { echo "FAIL: watch did not ingest the mid-run file"; exit 1; }

echo
echo "ALL E2E SCENARIOS PASSED"
