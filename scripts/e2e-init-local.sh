#!/usr/bin/env bash
# Manual E2E: run `suluctl init` against fresh copies of the 5 reference projects.
# Requires the external-test-frameworks-sulu repo at $FW_REPO (main — the glue was merged to main).
# Toolchains are NOT required — this asserts the scaffold, not a full test run.
set -euo pipefail

FW_REPO="${FW_REPO:-/Users/ruslan/projects/aisulu_dev/external-test-frameworks-sulu}"
HERE="$(cd "$(dirname "$0")/.." && pwd)"

go build -o /tmp/suluctl "$HERE"

run_one() {
  local name="$1" srcdir="$2" gluecheck="$3"; shift 3
  echo "=== $name ==="
  local work; work="$(mktemp -d)"
  cp -R "$FW_REPO/$srcdir/." "$work/"
  # Strip pre-existing glue to simulate a clean project.
  rm -rf "$work/src/test/java/ai/sulu" "$work/tests/support/sulu.ts" \
         "$work/sulu_pytest.py" "$work/Support/SuluTestAttribute.cs" 2>/dev/null || true

  ( cd "$work" && /tmp/suluctl init "$@" )
  if [ ! -e "$work/$gluecheck" ]; then
    echo "FAIL: expected glue $gluecheck not created" >&2; exit 1
  fi
  echo "OK: $name glue present at $gluecheck"
  rm -rf "$work"
}

run_one testng     java-testng-external-sulu-test   "src/test/java/sulu/SuluLabelListener.java"   --framework testng     --package sulu
run_one junit5     java-junit5-external-sulu-test    "src/test/java/sulu/SuluAllureExtension.java" --framework junit5     --package sulu
run_one playwright js-playwright-external-sulu-test  "tests/support/sulu.ts"                       --framework playwright
run_one pytest     python-pytest-external-sulu-test  "sulu_pytest.py"                              --framework pytest
run_one xunit      dotnet-xunit-external-sulu-test   "Support/SuluTestAttribute.cs"                --framework xunit

echo
echo "All init scaffolds produced glue. Reminder: run the printed 'suluctl watch ...' against a"
echo "live Sulu to confirm linkage; xUnit + Playwright need their documented manual step first."
