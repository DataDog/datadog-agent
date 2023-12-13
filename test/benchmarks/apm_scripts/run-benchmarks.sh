#!/usr/bin/env bash

ARTIFACTS_DIR="/artifacts/${CI_JOB_ID}"
mkdir -p "${ARTIFACTS_DIR}"

inv trace-agent.benchmarks --output="${ARTIFACTS_DIR}/pr_bench.txt" --bench="BenchmarkAgentTraceProcessing$"

git checkout main
inv trace-agent.benchmarks --output="${ARTIFACTS_DIR}/main_bench.txt" --bench="BenchmarkAgentTraceProcessing$"

git checkout "${CI_COMMIT_REF_NAME}" # (Only needed while these changes aren't merged to main)
