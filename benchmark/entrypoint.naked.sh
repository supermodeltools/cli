#!/bin/bash
set -euo pipefail

RUN_TESTS="python tests/runtests.py --settings=test_sqlite change_tracking"

echo "============================================================"
echo "BENCHMARK: naked Claude Code — django/django"
echo "============================================================"
echo

echo "--- Initial test run (all 8 should FAIL/ERROR) ---"
cd /app
PYTHONPATH=tests $RUN_TESTS -v 0 2>&1 | tail -3 || true
echo

echo "--- Running Claude Code on task ---"
cd /app
claude \
  --print "$(cat /benchmark/task.md)" \
  --dangerously-skip-permissions \
  --output-format stream-json \
  --verbose \
  2>&1 | tee /tmp/claude_raw.txt

echo
echo "============================================================"
echo "TEST RESULTS"
echo "============================================================"
PYTHONPATH=tests $RUN_TESTS -v 2 2>&1 | tee /tmp/test_results.txt

echo
echo "============================================================"
echo "COST SUMMARY"
echo "============================================================"
grep '"costUSD"\|"total_cost_usd"' /tmp/claude_raw.txt 2>/dev/null | tail -3 || echo "(check log)"
