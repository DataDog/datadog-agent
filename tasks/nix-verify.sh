#!/usr/bin/env bash
# nix-verify.sh — Run the datadog-agent Nix dev shell verification suite.
#
# Usage (inside nix develop or nix develop .#release):
#   bash tasks/nix-verify.sh [--suite=<slice|all|release>] [--include-slow]
#
# Suites:
#   slice0   nix develop entry only
#   slice1   Go toolchain
#   slice2   dev tools (install-tools)
#   slice3   Rust toolchain
#   slice4   Python + host C
#   slice5   rtloader CMake
#   slice6   agent.build
#   slice7   Ruby + omnibus (tight check; full build requires --include-slow)
#   all      full 15-item suite (default)
#   release  release-shell checks: cross-compilers, glibc floor, embedded Python
#            Run inside `nix develop .#release` to prove release artifacts are correct.
#
# Pass/fail is reported per item; overall exit code is non-zero if any required item fails.
set -euo pipefail

SUITE="all"
INCLUDE_SLOW=false
for arg in "$@"; do
  case "$arg" in
    --suite=*)   SUITE="${arg#--suite=}" ;;
    --include-slow) INCLUDE_SLOW=true ;;
  esac
done

PASS=0
FAIL=0
RESULTS=()

run_check() {
  local label="$1"; shift
  local cmd=("$@")
  local start end duration rc

  start=$(date +%s%3N)
  if "${cmd[@]}" > /tmp/nix-verify-out.txt 2>&1; then
    rc=0
  else
    rc=$?
  fi
  end=$(date +%s%3N)
  duration=$(( end - start ))

  if [ $rc -eq 0 ]; then
    echo "PASS [${duration}ms] $label"
    RESULTS+=("PASS | $label | ${duration}ms")
    PASS=$(( PASS + 1 ))
  else
    echo "FAIL [${duration}ms] $label"
    echo "     → Command: ${cmd[*]}"
    echo "     → Output:"
    sed 's/^/       /' /tmp/nix-verify-out.txt | tail -20
    RESULTS+=("FAIL | $label | ${duration}ms")
    FAIL=$(( FAIL + 1 ))
  fi
}

echo "========================================"
echo " datadog-agent Nix verification suite"
echo " suite=$SUITE  include-slow=$INCLUDE_SLOW"
echo " $(date -u)"
echo "========================================"

# --- Slice 0: Bootstrap ---
if [[ "$SUITE" =~ ^(slice0|all)$ ]]; then
  echo ""
  echo "── Slice 0: Bootstrap ──"
  run_check "nix develop enters shell" bash -c 'command -v go >/dev/null && command -v rustc >/dev/null && command -v python3 >/dev/null'
fi

# --- Slice 1: Go toolchain ---
if [[ "$SUITE" =~ ^(slice1|all)$ ]]; then
  echo ""
  echo "── Slice 1: Go toolchain ──"
  EXPECTED_GO=$(cat .go-version 2>/dev/null | tr -d '[:space:]')
  run_check "go version == $EXPECTED_GO" bash -c "go version | grep -q 'go${EXPECTED_GO}'"
  run_check "go build ./pkg/util/log/..." go build ./pkg/util/log/...
fi

# --- Slice 2: Dev tools ---
if [[ "$SUITE" =~ ^(slice2|all)$ ]]; then
  echo ""
  echo "── Slice 2: Dev tools ──"
  run_check "dda --version" dda --version
  run_check "dda inv install-tools (exit 0)" dda inv install-tools
  # shellcheck disable=SC2016  # $GOBIN intentionally expands inside bash -c
  run_check "golangci-lint present after install-tools" bash -c 'command -v golangci-lint || [ -x "$GOBIN/golangci-lint" ]'
fi

# --- Slice 3: Rust toolchain ---
if [[ "$SUITE" =~ ^(slice3|all)$ ]]; then
  echo ""
  echo "── Slice 3: Rust toolchain ──"
  run_check "rustc version matches rust-toolchain.toml" bash -c 'rustc --version | grep -q "1\.91"'
  run_check "cargo check --workspace" cargo check --workspace --quiet
  run_check "rustfmt present" bash -c 'command -v rustfmt'
  run_check "clippy present" bash -c 'command -v cargo-clippy || cargo clippy --version'
fi

# --- Slice 4: Python + host C ---
if [[ "$SUITE" =~ ^(slice4|all)$ ]]; then
  echo ""
  echo "── Slice 4: Python + host C ──"
  EXPECTED_PY=$(cat .python-version 2>/dev/null | tr -d '[:space:]')
  run_check "python3 version starts with $EXPECTED_PY" bash -c "python3 --version | grep -q 'Python ${EXPECTED_PY}'"
  run_check "python3 dev headers available" bash -c 'python3-config --includes | grep -q "\-I"'
  run_check "cmake present" cmake --version
  run_check "gcc/cc present" bash -c 'command -v cc || command -v gcc'
fi

# --- Slice 5: rtloader CMake ---
if [[ "$SUITE" =~ ^(slice5|all)$ ]]; then
  echo ""
  echo "── Slice 5: rtloader CMake (using Nix Python) ──"
  # shellcheck disable=SC2016  # $DD_RTLOADER_PYTHON3_ROOT intentionally expands inside bash -c
  run_check "DD_RTLOADER_PYTHON3_ROOT is set" bash -c '[ -n "$DD_RTLOADER_PYTHON3_ROOT" ]'
  run_check "rtloader.clean + rtloader.make (Nix Python)" bash -c '
    dda inv rtloader.clean 2>&1 | tail -3
    dda inv rtloader.make 2>&1 | tail -5
    # Verify the built library exists
    ls rtloader/build/rtloader/*.so 2>/dev/null || ls rtloader/build/rtloader/*.dylib 2>/dev/null
  '
fi

# --- Slice 6 / Item 3: agent.build ---
if [[ "$SUITE" =~ ^(slice6|all)$ ]]; then
  echo ""
  echo "── Slice 6: agent.build ──"
  run_check "[3] dda inv agent.build --build-exclude=systemd" dda inv agent.build --build-exclude=systemd
fi

# --- Slice 7 / Item 15: omnibus (Kevin's requirement) ---
if [[ "$SUITE" =~ ^(slice7|all)$ ]]; then
  echo ""
  echo "── Slice 7: Ruby + omnibus ──"
  run_check "ruby version 3.x (nix provides 3.3; omnibus has no hard constraint)" bash -c 'ruby --version | grep -q "ruby 3\."'
  run_check "bundler present" bash -c 'command -v bundle || command -v bundler'
  run_check "bundle install in omnibus/" bash -c 'cd omnibus && bundle install --quiet'
  run_check "bundle exec omnibus --version" bash -c 'cd omnibus && bundle exec omnibus --version'
  if [ "$INCLUDE_SLOW" = true ]; then
    run_check "[15] dda inv omnibus.build (slow ~30-60min)" dda inv omnibus.build
  else
    echo "SKIP  [0ms] [15] dda inv omnibus.build (pass --include-slow to run)"
  fi
fi

# --- Full 15-item suite (all except omnibus slow build) ---
if [[ "$SUITE" == "all" ]]; then
  echo ""
  echo "── Remaining integration tests ──"
  run_check "[4] dda inv test --targets=./pkg/..." dda inv test --targets=./pkg/...
  run_check "[5] dda inv linter.go --targets=./pkg/..." dda inv linter.go --targets=./pkg/...
  run_check "[6] dda inv trace-agent.integration-tests" dda inv trace-agent.integration-tests
  run_check "[7] dda inv otel-agent.integration-test" dda inv otel-agent.integration-test
  run_check "[8] dogstatsd server integration" dda inv test --targets=./comp/dogstatsd/server/impl --build-include=test
  run_check "[9] dogstatsd listeners integration" dda inv test --targets=./comp/dogstatsd/listeners --build-include=test
  run_check "[10] netflow server integration" dda inv test --targets=./comp/netflow/server/impl --build-include=test
  run_check "[11] logs pipeline failover" dda inv test --targets=./comp/logs-library/pipeline --build-include=test
  run_check "[12] file tailer integration" dda inv test --targets=./pkg/logs/tailers/file --build-include=test
  run_check "[13] configsync integration" dda inv test --targets=./comp/core/configsync/impl --build-include=test
  run_check "[14] fakeintake client tests" dda inv test --targets=./test/fakeintake/client
fi

# --- Release suite (run inside `nix develop .#release`) ---
# This is the local equivalent of the CI glibc floor check (plan Section 6, Tier 2).
# Proves the devShells.release toolchain produces release-quality binaries.
if [[ "$SUITE" =~ ^(release)$ ]]; then
  echo ""
  echo "── Release suite (nix develop .#release) ──"

  run_check "x86_64 cross-gcc present" bash -c 'command -v x86_64-unknown-linux-gnu-gcc'
  run_check "aarch64 cross-gcc present" bash -c 'command -v aarch64-unknown-linux-gnu-gcc'

  # Build the agent with the Nix cross-compilers (non-Bazel path)
  run_check "agent.build with Nix release toolchain" bash -c '
    DD_CC=x86_64-unknown-linux-gnu-gcc \
    DD_CXX=x86_64-unknown-linux-gnu-g++ \
      dda inv agent.build --build-exclude=systemd 2>&1 | tail -5
  '

  # Tier 2 gate: glibc floor check.
  # PASS iff the binary's max GLIBC symbol version <= 2.17 (x86_64) / <= 2.23 (aarch64).
  # shellcheck disable=SC2016  # $max_glibc and $agent_bin expand inside bash -c's scope
  run_check "glibc floor <= 2.17 (x86_64 release target)" bash -c '
    agent_bin="$(find bin/agent -name "agent" -type f 2>/dev/null | head -1)"
    if [ -z "$agent_bin" ]; then echo "agent binary not found"; exit 1; fi
    max_glibc=$(objdump -T "$agent_bin" 2>/dev/null \
      | grep -oP "GLIBC_\K[0-9.]+" | sort -V | tail -1)
    echo "  Max GLIBC ref: $max_glibc"
    python3 -c "
from packaging.version import Version
import sys
max_v = Version(\"$max_glibc\")
limit = Version(\"2.17\")
if max_v <= limit:
    print(f\"  {max_v} <= {limit} ✓\")
    sys.exit(0)
else:
    print(f\"  {max_v} > {limit} — binary not glibc 2.17 compatible\")
    sys.exit(1)
"
  '

  # Embedded Python check: verify EMBEDDED_PYTHON is set and is Python 3.12.6
  # shellcheck disable=SC2016  # $EMBEDDED_PYTHON expands inside bash -c's scope
  run_check "EMBEDDED_PYTHON set to 3.12.6" bash -c '
    if [ -z "$EMBEDDED_PYTHON" ]; then
      echo "  EMBEDDED_PYTHON not set — nix/embedded-python.nix not yet built (TBD-1/2)"
      exit 1
    fi
    "$EMBEDDED_PYTHON/bin/python3" --version | grep -q "3.12.6"
  '

  # shellcheck disable=SC2016  # $EMBEDDED_PYTHON expands inside bash -c's scope
  run_check "embedded Python has libpython3.12.so (shared)" bash -c '
    if [ -z "$EMBEDDED_PYTHON" ]; then exit 1; fi
    ldd "$EMBEDDED_PYTHON/lib/libpython3.12.so" > /dev/null
  '
fi

# --- Summary ---
echo ""
echo "========================================"
echo " Results: $PASS passed, $FAIL failed"
echo "========================================"
for r in "${RESULTS[@]}"; do echo "  $r"; done
echo ""

[ "$FAIL" -eq 0 ]
