#!/usr/bin/env bash

set -euo pipefail

bucket="${OBSERVER_ABLATION_ARTIFACT_BUCKET:-observer-log-ad-eval-artifacts-ddbuild}"
manifest_path="${OBSERVER_ABLATION_TESTBENCH_MANIFEST:-observer-ddeval-testbench.env}"
metadata_path="${OBSERVER_ABLATION_TESTBENCH_METADATA:-observer-ddeval-testbench.json}"
testbench_uri="${OBSERVER_ABLATION_TESTBENCH_URI:-}"
testbench_sha256="${OBSERVER_ABLATION_TESTBENCH_SHA256:-}"
testbench_bytes=""

if [[ -n "$testbench_uri" || -n "$testbench_sha256" ]]; then
    if [[ -z "$testbench_uri" || -z "$testbench_sha256" ]]; then
        echo "Set both OBSERVER_ABLATION_TESTBENCH_URI and OBSERVER_ABLATION_TESTBENCH_SHA256, or neither." >&2
        exit 1
    fi
else
    : "${CI_PROJECT_DIR:?CI_PROJECT_DIR is required to build the testbench}"
    : "${CI_COMMIT_SHA:?CI_COMMIT_SHA is required to publish the testbench}"

    export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
    dda inv anomalydetection.build-testbench

    testbench="$CI_PROJECT_DIR/bin/anomalydetection-testbench"
    test -x "$testbench"
    file "$testbench"

    testbench_sha256="$(sha256sum "$testbench" | awk '{print $1}')"
    timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
    key="test-binaries/${CI_COMMIT_SHA}/${timestamp}/linux-amd64/anomalydetection-testbench"
    testbench_uri="s3://${bucket}/${key}"
    testbench_bytes="$(wc -c < "$testbench" | tr -d ' ')"

    aws s3 cp --only-show-errors --region us-east-1 --sse AES256 "$testbench" "$testbench_uri"
    remote_size="$(aws s3api head-object --region us-east-1 --bucket "$bucket" --key "$key" --query ContentLength --output text)"
    test "$remote_size" = "$testbench_bytes"
fi

if [[ ! "$testbench_sha256" =~ ^[0-9a-f]{64}$ ]]; then
    echo "OBSERVER_ABLATION_TESTBENCH_SHA256 must be a lowercase SHA-256 digest." >&2
    exit 1
fi

printf 'OBSERVER_ABLATION_TESTBENCH_URI=%s\n' "$testbench_uri" > "$manifest_path"
printf 'OBSERVER_ABLATION_TESTBENCH_SHA256=%s\n' "$testbench_sha256" >> "$manifest_path"
jq -n \
    --arg uri "$testbench_uri" \
    --arg sha256 "$testbench_sha256" \
    --arg commit "${CI_COMMIT_SHA:-}" \
    --arg bytes "$testbench_bytes" \
    '{testbench: {
        uri: $uri,
        sha256: $sha256,
        commit: $commit,
        bytes: (if $bytes == "" then null else ($bytes | tonumber) end)
    }}' \
    > "$metadata_path"

cat "$metadata_path"
