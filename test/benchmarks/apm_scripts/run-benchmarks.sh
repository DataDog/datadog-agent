#!/usr/bin/env bash

set -e

bench_loop_x10 () {
  for i in {1..10}; do
    inv trace-agent.benchmarks --output="/tmp/$1" --bench="BenchmarkAgentTraceProcessing$"
    cat "/tmp/$1" >> "$ARTIFACTS_DIR/$1"
  done
}

bench_loop_x10 "pr_bench.txt"
git checkout main && bench_loop_x10 "main_bench.txt"

git checkout "${CI_COMMIT_REF_NAME}" # (Only needed while these changes aren't merged to main)
