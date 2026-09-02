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

# One start of serverless-init, returning the config-load duration in ms.
#
# Passing a command puts serverless-init in init mode, where it spawns that
# process and exits once it finishes, so `exit 0` gives a full init-then-exit
# cycle that returns on its own.
#
# DD_API_KEY must be non-empty or main.go disables DogStatsD and changes the
# startup path; the value is never used. DD_LOG_LEVEL=debug is required because
# the duration is reported at debug level.
measure_once() {
    DD_API_KEY=placeholder DD_LOG_LEVEL=debug \
        run_bounded "$BINARY" /bin/sh -c 'exit 0' 2>&1 |
        sed -n 's/.*config load took \([0-9.]*\)ms.*/\1/p' |
        head -1
}

echo "goos: $(go env GOOS)"
echo "goarch: $(go env GOARCH)"
echo "pkg: github.com/DataDog/datadog-agent/cmd/serverless-init"

# Discarded: the first start of a freshly built binary pays cold page-cache
# costs on a ~77MB executable and reads high.
echo "warmup (discarded)" >&2
measure_once >/dev/null

for i in $(seq 1 "${ITERATION_COUNT}"); do
    ms=$(measure_once)
    if [ -z "$ms" ]; then
        echo "ERROR: no config-load duration on iteration $i." >&2
        echo "The binary must be built from a commit containing the instrumentation" >&2
        echo "in cmd/serverless-init/main.go, and DD_LOG_LEVEL=debug must reach it." >&2
        exit 1
    fi
    echo "iteration $i: ${ms}ms" >&2
    # benchstat wants ns; emit one sample per line as a 1-iteration benchmark.
    awk -v ms="$ms" 'BEGIN { printf "BenchmarkConfigLoad 1 %d ns/op\n", ms * 1000000 }'
done
