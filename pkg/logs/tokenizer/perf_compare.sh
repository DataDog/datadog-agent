#!/bin/bash
# Compares the tokenizer's consumer benchmarks between a base revision and the
# working tree, on this machine. Exits 1 if the tokenizer got slower or
# allocates more, and 2 if the comparison could not run.
#
#   pkg/logs/tokenizer/perf_compare.sh [base]    # default: merge base with origin/main
#
# Both sides run this tree's consumer_benchmark_test.go (BenchmarkConsumer and
# BenchmarkConsumerMatch) against their own tokenizer sources, so only the
# tokenizer differs. Runs alternate between the two sides so machine noise hits
# both. The run fails when benchstat finds a benchmark more than
# PERF_MAX_SLOWDOWN percent slower (default 5) at p < 0.05, when the geomean of
# all of them is more than PERF_MAX_SLOWDOWN percent slower (benchstat gives it
# no p-value), or when a benchmark's median allocs/op goes up. Needs benchstat
# on PATH (dda inv install-tools).
#
# Tune with PERF_MAX_SLOWDOWN, PERF_ROUNDS (default 10; at least 4, since fewer
# cannot reach p < 0.05), PERF_BENCHTIME (default 200ms) and PERF_BENCH
# (default ^BenchmarkConsumer).
set -euo pipefail

fail() {
  echo "$*" >&2
  exit 2
}

pkg=pkg/logs/tokenizer
root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
base=${1:-$(git -C "$root" merge-base HEAD origin/main)}
max_slowdown=${PERF_MAX_SLOWDOWN:-5}
rounds=${PERF_ROUNDS:-10}
benchtime=${PERF_BENCHTIME:-200ms}
bench=${PERF_BENCH:-^BenchmarkConsumer}
[[ $rounds =~ ^[0-9]+$ ]] && [ "$rounds" -ge 4 ] || fail "PERF_ROUNDS must be a whole number of at least 4, got '$rounds'"
[[ $max_slowdown =~ ^[0-9]+(\.[0-9]+)?$ ]] || fail "PERF_MAX_SLOWDOWN must be a number of percent like 5, got '$max_slowdown'"
command -v benchstat > /dev/null || fail "benchstat not found: run dda inv install-tools"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# The tokenizer imports only the standard library, so each side builds as a
# throwaway module holding just the package's sources and this tree's
# benchmarks.
go_line=$(awk '/^go /{print; exit}' "$root/go.mod")
mkdir -p "$work/base" "$work/head/$pkg"
git -C "$root" archive "$base" "$pkg" | tar -x -C "$work/base" || fail "cannot read $pkg at $base"
cp "$root/$pkg"/*.go "$work/head/$pkg/"
for side in base head; do
  printf 'module github.com/DataDog/datadog-agent\n\n%s\n' "$go_line" > "$work/$side/go.mod"
  find "$work/$side/$pkg" -name '*_test.go' -delete
  rm -rf "$work/$side/$pkg/gentokentables"
  cp "$root/$pkg/consumer_benchmark_test.go" "$work/$side/$pkg/"
  # -trimpath: identical sources give identical binaries, so code layout cannot differ.
  (cd "$work/$side" && CGO_ENABLED=0 GOWORK=off go test -c -trimpath -o "$work/$side.test" "./$pkg") ||
    fail "cannot build the $side side: does $base have the API consumer_benchmark_test.go uses?"
done

echo "Comparing $(git -C "$root" rev-parse --short "$base") with the working tree at $root: $rounds rounds of $bench"
for _ in $(seq "$rounds"); do
  for side in base head; do
    "$work/$side.test" -test.run '^$' -test.bench "$bench" -test.benchmem \
      -test.benchtime "$benchtime" -test.cpu 1 >> "$work/$side.txt" ||
      { tail -20 "$work/$side.txt" >&2; fail "the $side benchmarks failed"; }
  done
done
grep -q '^Benchmark' "$work/head.txt" || fail "no benchmark matches $bench: nothing was compared"

benchstat base="$work/base.txt" head="$work/head.txt"
benchstat -format csv base="$work/base.txt" head="$work/head.txt" 2> /dev/null |
  awk -F, -v max="$max_slowdown" '
    $1 == "" && $2 != "base" { unit = $2; next }
    unit == "" || $1 == "" { next }
    # Allocation counts are exact, so compare medians. benchstat prints "?"
    # instead of a percentage when the base is 0.
    unit == "allocs/op" && $1 != "geomean" && $4 + 0 > $2 + 0 {
      printf "REGRESSION %s allocs/op: %s -> %s\n", $1, $2, $4; failed = 1
    }
    unit == "sec/op" && $6 ~ /^\+/ {
      delta = $6; sub(/^\+/, "", delta); sub(/%$/, "", delta)
      if (delta + 0 > max + 0) { printf "REGRESSION %s sec/op: %s\n", $1, $6; failed = 1 }
    }
    END { exit failed }' ||
  { echo "FAIL: the tokenizer got slower or allocates more (see REGRESSION lines)"; exit 1; }
echo "PASS: no benchmark is more than $max_slowdown% slower or allocates more"
