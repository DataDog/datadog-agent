#!/usr/bin/env bash

inv trace-agent.benchmarks --output="$ARTIFACTS_DIR/pr_bench.txt" --bench="BenchmarkAgentTraceProcessing$"
git checkout main

inv trace-agent.benchmarks --output="$ARTIFACTS_DIR/main_bench.txt" --bench="BenchmarkAgentTraceProcessing$"
git checkout "${CI_COMMIT_REF_NAME}" # (Only needed while these changes aren't merged to main)
