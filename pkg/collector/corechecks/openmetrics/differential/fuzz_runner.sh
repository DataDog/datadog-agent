#!/bin/bash
# Background-friendly fuzz runner.
#
# Default `go test -fuzz` halts on first failure, which is the wrong default
# for a long unattended hunt: each finding produces ONE input we have to
# manually triage before the engine resumes. With the known-divergence
# classifier (see known_divergences.go) most findings will be skipped, but
# any new bug class will still stop the run.
#
# This script loops: each iteration runs the fuzz for the remaining budget,
# and when it stops with a failing input, moves that input out of the
# corpus dir (so the next iteration ignores it) into
# `/tmp/fuzz-findings/<timestamp>-<basename>` and resumes. The accumulated
# findings can be triaged in-batch later.
#
# Usage:  ./fuzz_runner.sh [HOURS]
set -euo pipefail

HOURS="${1:-8}"

# Find repo root by walking up looking for `go.mod` — robust to whoever calls
# the script from where.
SCRIPT_DIR="$(dirname "$(realpath "$0")")"
REPO_ROOT="$SCRIPT_DIR"
while [ "$REPO_ROOT" != "/" ] && [ ! -f "$REPO_ROOT/go.mod" ]; do
  REPO_ROOT="$(dirname "$REPO_ROOT")"
done
if [ ! -f "$REPO_ROOT/go.mod" ]; then
  echo "fuzz_runner: could not locate repo root (no go.mod found walking up from $SCRIPT_DIR)" >&2
  exit 1
fi
cd "$REPO_ROOT"
echo "fuzz_runner: cwd=$REPO_ROOT"

PKG=./pkg/collector/corechecks/openmetrics/differential
FINDINGS_DIR=/tmp/fuzz-findings
mkdir -p "$FINDINGS_DIR"

FUZZ_CACHE="$REPO_ROOT/pkg/collector/corechecks/openmetrics/differential/testdata/fuzz/FuzzOpenMetricsDifferential"

DEADLINE=$(($(date +%s) + HOURS * 3600))
iter=0
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
  iter=$((iter + 1))
  REMAINING=$((DEADLINE - $(date +%s)))
  if [ "$REMAINING" -lt 60 ]; then break; fi
  echo "=== fuzz iteration $iter, ${REMAINING}s remaining ==="
  set +e
  go test -tags openmetrics_differential -run=^$ \
      -fuzz=FuzzOpenMetricsDifferential \
      -fuzztime=${REMAINING}s \
      -timeout=$(( REMAINING + 60 ))s \
      "$PKG"
  rc=$?
  set -e
  if [ "$rc" -eq 0 ]; then
    echo "=== fuzz iteration $iter completed cleanly ==="
    break
  fi
  # Move any newly-committed failing input out of the corpus into findings.
  if [ -d "$FUZZ_CACHE" ]; then
    moved=0
    for f in "$FUZZ_CACHE"/*; do
      [ -e "$f" ] || continue
      stamp=$(date +%Y%m%d-%H%M%S)
      mv "$f" "$FINDINGS_DIR/${stamp}-$(basename "$f")"
      moved=$((moved + 1))
      echo "=== finding moved to $FINDINGS_DIR/${stamp}-$(basename "$f") ==="
    done
    if [ "$moved" -eq 0 ]; then
      # No moveable finding: fuzz probably failed for a non-finding reason
      # (build error, sidecar startup failure). Don't busy-loop — give up.
      echo "fuzz_runner: fuzz failed with no movable finding; aborting" >&2
      exit "$rc"
    fi
  else
    echo "fuzz_runner: cache dir $FUZZ_CACHE not found after failure; aborting" >&2
    exit "$rc"
  fi
done

echo "=== fuzz run complete; findings in $FINDINGS_DIR ==="
ls -la "$FINDINGS_DIR" || true
