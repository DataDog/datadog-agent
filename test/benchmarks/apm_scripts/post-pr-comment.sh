#!/usr/bin/env bash

REPORTS_DIR="$(pwd)/reports/"

cat ${REPORTS_DIR}/report.md | /usr/local/bin/pr-commenter --for-repo="$CI_PROJECT_NAME" --for-pr="$CI_COMMIT_REF_NAME" --header="Benchmarks"
