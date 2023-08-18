#!/usr/bin/env bash

ARTIFACTS_DIR="/artifacts/${CI_JOB_ID}"
REPORTS_DIR="$(pwd)/reports/"
mkdir "${REPORTS_DIR}" || :

source /benchmark-analyzer/.venv/bin/activate
cd /benchmark-analyzer

./benchmark_analyzer convert \
  --framework=GoBench \
  --outpath="pr.json" \
  --extra-params="{\"trace_agent\":\"${CI_COMMIT_REF_NAME}\"}" \
  "${ARTIFACTS_DIR}/pr_bench.txt"

./benchmark_analyzer convert \
  --framework=GoBench \
  --outpath="main.json" \
  --extra-params="{\"trace_agent\":\"main\"}" \
  "${ARTIFACTS_DIR}/main_bench.txt"

./benchmark_analyzer compare pairwise --outpath ${REPORTS_DIR}/report.md --format md-nodejs main.json pr.json
./benchmark_analyzer compare pairwise --outpath ${REPORTS_DIR}/report_full.html --format html main.json pr.json

