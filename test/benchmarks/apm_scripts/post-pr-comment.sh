#!/usr/bin/env bash

cat $ARTIFACTS_DIR/report.md | pr-commenter --for-repo="$CI_PROJECT_NAME" --for-pr="$CI_COMMIT_REF_NAME" --header="Benchmarks"
