#!/bin/bash
# Usage: ./benchmark/run.sh
# Requires: ANTHROPIC_API_KEY, SUPERMODEL_API_KEY

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
RESULTS_DIR="$SCRIPT_DIR/results"

# ── Preflight checks ──────────────────────────────────────────────────────────

if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "error: ANTHROPIC_API_KEY is not set" >&2
  exit 1
fi

if [[ -z "${SUPERMODEL_API_KEY:-}" ]]; then
  echo "error: SUPERMODEL_API_KEY is not set (needed for supermodel container)" >&2
  exit 1
fi

mkdir -p "$RESULTS_DIR"

# ── Build images ──────────────────────────────────────────────────────────────

echo "==> Building bench-naked..."
docker build \
  -f "$SCRIPT_DIR/Dockerfile.naked" \
  -t bench-naked \
  "$SCRIPT_DIR" \
  2>&1 | tail -3

echo "==> Building bench-supermodel (builds supermodel from source)..."
docker build \
  -f "$SCRIPT_DIR/Dockerfile.supermodel" \
  -t bench-supermodel \
  "$REPO_ROOT" \
  2>&1 | tail -3

echo "==> Building bench-threefile (three-file shard format)..."
docker build \
  -f "$SCRIPT_DIR/Dockerfile.threefile" \
  -t bench-threefile \
  "$REPO_ROOT" \
  2>&1 | tail -3

echo

# ── Run containers ────────────────────────────────────────────────────────────

echo "==> Running naked container..."
docker run --rm \
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  bench-naked \
  2>&1 | tee "$RESULTS_DIR/naked.txt"

echo
echo "==> Running supermodel container..."
docker run --rm \
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  -e SUPERMODEL_API_KEY="$SUPERMODEL_API_KEY" \
  bench-supermodel \
  2>&1 | tee "$RESULTS_DIR/supermodel.txt"

echo
echo "==> Running three-file container..."
docker run --rm \
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  -e SUPERMODEL_API_KEY="$SUPERMODEL_API_KEY" \
  bench-threefile \
  2>&1 | tee "$RESULTS_DIR/threefile.txt"

echo
echo "==> Comparing results..."
"$SCRIPT_DIR/compare.sh" "$RESULTS_DIR/naked.txt" "$RESULTS_DIR/supermodel.txt" "$RESULTS_DIR/threefile.txt"
