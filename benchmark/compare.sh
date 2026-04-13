#!/bin/bash
# Usage: ./benchmark/compare.sh results/naked.txt results/supermodel.txt
# Can also be run standalone after a benchmark run.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAKED_LOG="${1:-$SCRIPT_DIR/results/naked.txt}"
SUPERMODEL_LOG="${2:-$SCRIPT_DIR/results/supermodel.txt}"

if [[ ! -f "$NAKED_LOG" ]]; then
  echo "error: naked log not found: $NAKED_LOG" >&2
  exit 1
fi
if [[ ! -f "$SUPERMODEL_LOG" ]]; then
  echo "error: supermodel log not found: $SUPERMODEL_LOG" >&2
  exit 1
fi

# ── Helpers ───────────────────────────────────────────────────────────────────

extract_tests() {
  local log="$1"
  # Django test runner outputs lines like:
  #   Ran 15 tests in 0.123s
  #   OK  or  FAILED (failures=2, errors=1)
  local ran errors failures
  ran=$(grep -oP 'Ran \K[0-9]+' "$log" 2>/dev/null | tail -1 || echo "?")
  failures=$(grep -oP 'failures=\K[0-9]+' "$log" 2>/dev/null | tail -1 || echo "0")
  errors=$(grep -oP 'errors=\K[0-9]+' "$log" 2>/dev/null | tail -1 || echo "0")
  local status="PASS"
  if grep -q 'FAILED' "$log" 2>/dev/null; then
    status="FAIL"
  fi
  echo "$ran tests | $status | failures=$failures errors=$errors"
}

extract_cost() {
  local log="$1"
  # Claude Code stream-json emits a final result object with costUSD.
  # Try several patterns in order of specificity.
  local cost
  cost=$(grep -oP '"costUSD"\s*:\s*\K[0-9.]+' "$log" 2>/dev/null | tail -1) && { echo "\$$cost"; return; }
  cost=$(grep -oP '"cost_usd"\s*:\s*\K[0-9.]+' "$log" 2>/dev/null | tail -1) && { echo "\$$cost"; return; }
  cost=$(grep -oP 'Total cost[^0-9]*\K[0-9.]+' "$log" 2>/dev/null | tail -1) && { echo "\$$cost"; return; }
  echo "(not found — check log for token counts)"
}

extract_tokens() {
  local log="$1"
  local input output
  input=$(grep -oP '"input_tokens"\s*:\s*\K[0-9]+' "$log" 2>/dev/null | \
    awk '{s+=$1} END {print s+0}')
  output=$(grep -oP '"output_tokens"\s*:\s*\K[0-9]+' "$log" 2>/dev/null | \
    awk '{s+=$1} END {print s+0}')
  echo "in=${input:-?} out=${output:-?}"
}

# ── Report ────────────────────────────────────────────────────────────────────

printf '\n'
printf '%-26s  %-20s  %-20s\n' "" "naked" "supermodel"
printf '%-26s  %-20s  %-20s\n' "$(printf '%0.s─' {1..26})" "$(printf '%0.s─' {1..20})" "$(printf '%0.s─' {1..20})"
printf '%-26s  %-20s  %-20s\n' "Tests"      "$(extract_tests "$NAKED_LOG")"     "$(extract_tests "$SUPERMODEL_LOG")"
printf '%-26s  %-20s  %-20s\n' "API cost"   "$(extract_cost "$NAKED_LOG")"      "$(extract_cost "$SUPERMODEL_LOG")"
printf '%-26s  %-20s  %-20s\n' "Tokens"     "$(extract_tokens "$NAKED_LOG")"    "$(extract_tokens "$SUPERMODEL_LOG")"
printf '\n'

# ── Diff feature test outcomes ────────────────────────────────────────────────

naked_priority_pass=$(grep -c 'PriorityFeature.*ok\|ok.*PriorityFeature' "$NAKED_LOG" 2>/dev/null || \
  (grep 'PriorityFeature' "$NAKED_LOG" | grep -c 'ok' || echo 0))
sm_priority_pass=$(grep -c 'PriorityFeature.*ok\|ok.*PriorityFeature' "$SUPERMODEL_LOG" 2>/dev/null || \
  (grep 'PriorityFeature' "$SUPERMODEL_LOG" | grep -c 'ok' || echo 0))

echo "Priority feature tests (naked):      $naked_priority_pass / 8 passing"
echo "Priority feature tests (supermodel): $sm_priority_pass / 8 passing"
echo

# ── Show cost delta ───────────────────────────────────────────────────────────

naked_cost=$(grep -oP '"costUSD"\s*:\s*\K[0-9.]+' "$NAKED_LOG" 2>/dev/null | tail -1 || echo "")
sm_cost=$(grep -oP '"costUSD"\s*:\s*\K[0-9.]+' "$SUPERMODEL_LOG" 2>/dev/null | tail -1 || echo "")

if [[ -n "$naked_cost" && -n "$sm_cost" ]]; then
  python3 - <<PYEOF
naked = float("$naked_cost")
sm    = float("$sm_cost")
delta = naked - sm
pct   = (delta / naked * 100) if naked > 0 else 0
sign  = "cheaper" if delta > 0 else "more expensive"
print(f"supermodel was \${abs(delta):.4f} ({abs(pct):.1f}%) {sign} than naked")
PYEOF
fi

echo
echo "Full logs:"
echo "  naked:      $NAKED_LOG"
echo "  supermodel: $SUPERMODEL_LOG"
