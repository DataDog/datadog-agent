#!/bin/bash

# Measures how long serverless-init spends initializing its configuration, by
# starting the real binary repeatedly and reading the duration it reports.
#
# Why this phase: every config setting datadog-agent declares without the
# `full-agent-only:true` schema tag is compiled into serverless-init and costs
# work on every cold start. That surface churns constantly upstream, and the
# `full-agent-only` split exists to keep this phase small.
#
# Why the real binary rather than a Go benchmark: config declaration runs in
# pkg/config/setup's package init(), before main() is entered, so no
# serverless-init entry point reaches it. An in-process benchmark has to
# re-invoke it synthetically, which both understates the cost (~3.2ms synthetic
# vs ~6.8ms in a real cold process) and risks measuring accumulated global
# state — fixupInitConfig appends to a package-level override slice on each
# call, which LoadDatadog then replays. Starting a fresh process avoids that.
#
# Samples go to stdout in Go benchmark format so benchstat can compare two runs;
# progress goes to stderr. No threshold here on purpose: the workflow compares
# the PR against its base branch, so absolute machine speed cancels out.
#
# Usage:
#   test/integration/serverless_perf/compute.sh <path-to-binary> > samples.log
#   ITERATION_COUNT=51 test/integration/serverless_perf/compute.sh ./serverless-init

set -o pipefail

BINARY="${1:-/tmp/serverless-init}"

# Each start is a fresh process (~2.6s), so this trades wall-clock for
# precision. benchstat reported a ±42% confidence interval at 6 samples, and
# intervals shrink as 1/sqrt(n): ~±21% at 25, ~±10% at 100. A meaningful
# regression here is on the order of 10%, so 25 would leave the interval wider
# than the effect and benchstat would report "~" for a real change.
#
# Not worth pushing much past this: baseline and current run on different
# runners, and between-machine speed differences are systematic bias that more
# samples do not reduce.
ITERATION_COUNT="${ITERATION_COUNT:-100}"

if [ ! -x "$BINARY" ]; then
    echo "ERROR: serverless-init binary not found or not executable: $BINARY" >&2
    exit 1
fi

# Bound each run so a hang fails the job instead of stalling the workflow.
# perl is the fallback for local runs on macOS, which has no GNU timeout.
if command -v timeout >/dev/null 2>&1; then
    run_bounded() { timeout 30 "$@"; }
else
    run_bounded() { perl -e 'alarm 30; exec @ARGV' -- "$@"; }
fi

# One start of serverless-init, returning its whole output. Both durations are
# read from a single run: config load is the phase that grows per setting, and
# startup is everything before the user's workload, so config load as a share of
# startup comes free from the same samples.
#
# Passing a command puts serverless-init in init mode, where it spawns that
# process and exits once it finishes, so `exit 0` gives a full init-then-exit
# cycle that returns on its own.
#
# DD_API_KEY must be non-empty or main.go disables DogStatsD and changes the
# startup path; the value is never used. DD_LOG_LEVEL=debug is required because
# both durations are reported at debug level.
measure_once() {
    DD_API_KEY=placeholder DD_LOG_LEVEL=debug \
        run_bounded "$BINARY" /bin/sh -c 'exit 0' 2>&1
}

extract() { sed -n "s/.*$1 took \([0-9.]*\)ms.*/\1/p" | head -1; }

echo "goos: $(go env GOOS)"
echo "goarch: $(go env GOARCH)"
echo "pkg: github.com/DataDog/datadog-agent/cmd/serverless-init"

# Discarded: the first start of a freshly built binary pays cold page-cache
# costs on a ~77MB executable and reads high.
echo "warmup (discarded)" >&2
measure_once >/dev/null

samples=()
startup_samples=()

for i in $(seq 1 "${ITERATION_COUNT}"); do
    out=$(measure_once)
    ms=$(printf '%s\n' "$out" | extract "config load")
    startup_ms=$(printf '%s\n' "$out" | extract "startup")

    if [ -z "$ms" ]; then
        echo "ERROR: no config-load duration on iteration $i." >&2
        echo "The binary must be built from a commit containing the instrumentation" >&2
        echo "in cmd/serverless-init/main.go, and DD_LOG_LEVEL=debug must reach it." >&2
        exit 1
    fi

    # benchstat wants ns; emit one sample per line as a 1-iteration benchmark.
    # Two benchmark names means benchstat reports a row for each.
    awk -v ms="$ms" 'BEGIN { printf "BenchmarkConfigLoad 1 %d ns/op\n", ms * 1000000 }'

    # Startup is optional so a binary predating that log line still yields
    # config-load data rather than failing the whole run.
    if [ -n "$startup_ms" ]; then
        startup_samples+=("$startup_ms")
        awk -v ms="$startup_ms" 'BEGIN { printf "BenchmarkStartup 1 %d ns/op\n", ms * 1000000 }'
        echo "iteration $i: config load ${ms}ms, startup ${startup_ms}ms" >&2
    else
        echo "iteration $i: config load ${ms}ms (no startup line)" >&2
    fi

    samples+=("$ms")
done

# Summary on stderr so it lands in the job log next to the iterations. benchstat
# reports the same medians from stdout, but having them here means the numbers
# are visible without downloading an artifact. sort -g and awk rather than bash
# arithmetic: these are floats.
median_of() {
    printf '%s\n' "$@" | sort -g | awk -v n="$#" '
        NR==int((n+1)/2) { lo=$1 }
        NR==int(n/2)+1   { hi=$1 }
        END              { printf "%.3f", (lo+hi)/2 }'
}

median=$(median_of "${samples[@]}")
min=$(printf '%s\n' "${samples[@]}" | sort -g | head -1)
max=$(printf '%s\n' "${samples[@]}" | sort -g | tail -1)
echo "config load median: ${median}ms (n=${#samples[@]}, min ${min}ms, max ${max}ms)" >&2

if [ "${#startup_samples[@]}" -gt 0 ]; then
    startup_median=$(median_of "${startup_samples[@]}")
    smin=$(printf '%s\n' "${startup_samples[@]}" | sort -g | head -1)
    smax=$(printf '%s\n' "${startup_samples[@]}" | sort -g | tail -1)
    echo "startup median:     ${startup_median}ms (n=${#startup_samples[@]}, min ${smin}ms, max ${smax}ms)" >&2
    # The ratio is what makes the config-load number interpretable: it says how
    # much of serverless-init's startup this phase accounts for.
    awk -v c="$median" -v s="$startup_median" \
        'BEGIN { if (s > 0) printf "config load is %.0f%% of startup\n", 100*c/s }' >&2
fi
