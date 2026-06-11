#!/usr/bin/env bash
# ABOUTME: Runs gremlins mutation testing on changed Go packages under pkg/.
# ABOUTME: Invoked by the mutation-testing GitLab job. Builds patched gremlins, posts advisory report.
set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 [options]

Options:
  --base-ref <ref>             Git ref to diff against (default: origin/main)
  --output <path>              Write markdown report here (default: mutation-report.md)
  --timeout-coefficient <n>    Gremlins per-mutation timeout multiplier (default: 5)
  --all                        Mutate every package under pkg/ (local use only)
  --no-dda                     Use 'go test' instead of 'dda inv test' (for canary modules without build tags)
EOF
}

BASE_REF="origin/main"
OUTPUT="mutation-report.md"
TIMEOUT_COEFFICIENT="5"
RUN_ALL=""
USE_DDA="1"
PKG_ROOT="pkg"
# Mutation unit cap: each package takes several minutes. Three keeps CI under 25 min.
MAX_MUTATION_UNITS="${MAX_MUTATION_UNITS:-3}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-ref)            BASE_REF="$2";            shift 2 ;;
    --output)              OUTPUT="$2";              shift 2 ;;
    --timeout-coefficient) TIMEOUT_COEFFICIENT="$2"; shift 2 ;;
    --all)                 RUN_ALL="1";              shift ;;
    --no-dda)              USE_DDA="";               shift ;;
    -h|--help)             usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

SCRIPT_DIR=".gitlab/mutation-testing"

# Resolve changed Go source packages under pkg/.
changed_files() {
  if [[ -n "$RUN_ALL" ]]; then
    find "$PKG_ROOT" -name '*.go' \
      -not -name '*_test.go' \
      -not -name '*.pb.go' \
      -not -name '*_vtproto.pb.go' \
      -not -name '*_gen.go' \
      -not -name '*_generated.go' \
      -not -path '*/vendor/*' \
      -not -path '*/mocks/*' 2>/dev/null
    return
  fi
  git diff --name-only --diff-filter=ACMR "$BASE_REF...HEAD" -- "$PKG_ROOT/**/*.go" \
    | grep -v '_test\.go$' \
    | grep -v '\.pb\.go$' \
    | grep -v '_vtproto\.pb\.go$' \
    | grep -v '_gen\.go$' \
    | grep -v '_generated\.go$' \
    | grep -v '/vendor/' \
    | grep -v '/mocks/' \
    || true
}

mapfile -t FILES < <(changed_files)

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "No Go source files changed under $PKG_ROOT/. Nothing to mutate; no report produced."
  exit 0
fi

# Dedupe to distinct parent directories (Go packages).
declare -A PKG_SET
for f in "${FILES[@]}"; do
  PKG_SET["$(dirname "$f")"]=1
done
mapfile -t PACKAGES < <(printf '%s\n' "${!PKG_SET[@]}" | sort)

# Skip packages that require non-default build tags. Detect by attempting
# `go list -tags=''` from the module root; if it fails the package needs
# tags we can't satisfy in plain CI (system-probe, ebpf, kubelet, ...).
FILTERED_PACKAGES=()
for pkg in "${PACKAGES[@]}"; do
  if (cd "$pkg" && go list -tags='' ./... >/dev/null 2>&1); then
    FILTERED_PACKAGES+=("$pkg")
  else
    echo "Skipping $pkg: requires non-default build tags."
  fi
done

UNITS=${#FILTERED_PACKAGES[@]}
if [[ "$UNITS" -eq 0 ]]; then
  echo "All changed packages require non-default build tags; nothing to mutate. No comment will be posted."
  exit 0
fi

if [[ -z "$RUN_ALL" && "$UNITS" -gt "$MAX_MUTATION_UNITS" ]]; then
  echo "Too many mutation units changed ($UNITS > $MAX_MUTATION_UNITS). Skipping to keep CI fast; no report produced."
  exit 0
fi

# Build the patched gremlins from upstream + dd-source patch. This is
# required because stock gremlins's coverage gatherer fails on datadog-agent
# modules whose tests depend on -ldflags or build tags (the same symptom dd-source
# hit, different root cause). See .gitlab/mutation-testing/patches/.
GREMLINS_BIN="${GREMLINS_BIN:-$REPO_ROOT/.gitlab/mutation-testing/.gremlins}"
if [[ ! -x "$GREMLINS_BIN" ]]; then
  echo "Building patched gremlins -> $GREMLINS_BIN"
  GREMLINS_SRC="$(mktemp -d)"
  trap 'rm -rf "$GREMLINS_SRC"' EXIT
  git clone --depth 1 --branch v0.6.0 https://github.com/go-gremlins/gremlins.git "$GREMLINS_SRC" >/dev/null 2>&1
  (cd "$GREMLINS_SRC" && git apply "$REPO_ROOT/$SCRIPT_DIR/patches/0001-add-test-cmd-and-no-coverage-flags.patch")
  (cd "$GREMLINS_SRC" && go build -o "$GREMLINS_BIN" ./cmd/gremlins)
fi

echo "Running gremlins on $UNITS package(s):"
for p in "${FILTERED_PACKAGES[@]}"; do echo "  $p"; done

RESULTS_DIR="$(mktemp -d)"
trap 'rm -rf "$RESULTS_DIR" "${GREMLINS_SRC:-/dev/null}"' EXIT

packages_with_results=0
for pkg in "${FILTERED_PACKAGES[@]}"; do
  result_file="$RESULTS_DIR/$(echo "$pkg" | tr '/' '_').json"
  echo ""
  echo "=== gremlins unleash ./$pkg ==="

  # Test-command choice: dda inv (default, honors build tags and ldflags) or
  # plain go test (--no-dda escape hatch for canary modules without those needs).
  if [[ -n "$USE_DDA" ]]; then
    TEST_CMD="dda inv -- -e test --targets=./$pkg --skip-flakes"
  else
    TEST_CMD="go test"
  fi

  if (cd "$pkg" && "$GREMLINS_BIN" unleash \
        --silent \
        --no-coverage \
        --test-cmd "$TEST_CMD" \
        --output "$result_file" \
        --threshold-efficacy 0 \
        --threshold-mcover 0 \
        --timeout-coefficient "$TIMEOUT_COEFFICIENT" \
        ./) && [[ -s "$result_file" ]]; then
    packages_with_results=$((packages_with_results + 1))
  else
    echo "gremlins did not produce usable results for $pkg."
    rm -f "$result_file"
  fi
done

if [[ "$packages_with_results" -eq 0 ]]; then
  echo "No gremlins results produced for any package; no report produced."
  exit 0
fi

python3 "$SCRIPT_DIR/muttest_render.py" \
  --results-dir "$RESULTS_DIR" \
  --output "$OUTPUT"

echo "Report written to $OUTPUT"
exit 0
