#!/usr/bin/env bash

export UNCONFIDENCE_THRESHOLD=2.0

CI_JOB_DATE=$(date +%s)
CPU_MODEL=$(cat "$ARTIFACTS_DIR/lscpu.txt" | grep -Eo 'Model name: .*' | sed 's/Model name://' | awk '{$1=$1;print}')
if [ -z "$CPU_MODEL" ]; then
  echo "FAIL! Failed to extract CPU_MODEL from lscpu.txt"
  exit 1
fi

COMMIT_SHA=$(git rev-parse HEAD)
COMMIT_DATE=$(git show -s --format=%ct "$COMMIT_SHA")
benchmark_analyzer convert \
  --framework=GoBench \
  --outpath="pr.json" \
  --extra-params="{\
    \"baseline_or_candidate\":\"candidate\", \
    \"cpu_model\":\"$CPU_MODEL\", \
    \"ci_job_date\":\"$CI_JOB_DATE\", \
    \"ci_job_id\":\"$CI_JOB_ID\", \
    \"ci_pipeline_id\":\"$CI_PIPELINE_ID\", \
    \"git_commit_sha\":\"$COMMIT_SHA\", \
    \"git_commit_date\":\"$COMMIT_DATE\", \
    \"git_branch\":\"$CI_COMMIT_REF_NAME\"\
  }" \
  "$ARTIFACTS_DIR/pr_bench.txt"

git checkout "${BASE_BRANCH:-main}"
COMMIT_SHA=$(git rev-parse HEAD)
COMMIT_DATE=$(git show -s --format=%ct "$COMMIT_SHA")
benchmark_analyzer convert \
  --framework=GoBench \
  --outpath="base.json" \
  --extra-params="{\
    \"baseline_or_candidate\":\"baseline\", \
    \"cpu_model\":\"$CPU_MODEL\", \
    \"ci_job_date\":\"$CI_JOB_DATE\", \
    \"ci_job_id\":\"$CI_JOB_ID\", \
    \"ci_pipeline_id\":\"$CI_PIPELINE_ID\", \
    \"git_commit_sha\":\"$COMMIT_SHA\", \
    \"git_commit_date\":\"$COMMIT_DATE\", \
    \"git_branch\":\"${BASE_BRANCH:-main}\"\
  }" \
  "$ARTIFACTS_DIR/base_bench.txt"

benchmark_analyzer compare pairwise --outpath "$ARTIFACTS_DIR/report.md" --format md-nodejs base.json pr.json
benchmark_analyzer compare pairwise --outpath "$ARTIFACTS_DIR/report_full.html" --format html base.json pr.json

git checkout "${CI_COMMIT_REF_NAME}" # (Only needed while these changes aren't merged to main)
