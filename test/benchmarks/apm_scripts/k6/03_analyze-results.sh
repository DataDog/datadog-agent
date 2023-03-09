#!/usr/bin/env bash

ARTIFACTS_DIR="/artifacts/${CI_JOB_ID}"
REPORTS_DIR="$ARTIFACTS_DIR/reports/"
mkdir "${REPORTS_DIR}" || :

RUN_ID=k6-benchmark-dd-trace-agent

benchmark_analyzer convert \
  --framework=K6 \
  --outpath="pr.json" \
  --extra-params="{\"trace_agent\":\"${CI_COMMIT_REF_NAME}\"}" \
  "$ARTIFACTS_DIR/$RUN_ID.json"

RUN_ID=k6-benchmark-dd-trace-agent-baseline


benchmark_analyzer convert \
  --framework=K6 \
  --outpath="main.json" \
  --extra-params="{\"trace_agent\":\"main\"}" \
  "$ARTIFACTS_DIR/$RUN_ID.json"

./benchmark_analyzer compare pairwise --outpath ${REPORTS_DIR}/report.md --format md-nodejs main.json pr.json
./benchmark_analyzer compare pairwise --outpath ${REPORTS_DIR}/report_full.html --format html main.json pr.json

