#!/usr/bin/env bash
# Builds the rotation-loss demonstration binary on the host and stages it into this
# Docker build context. The Antithesis Go SDK requires CGO_ENABLED=1. The
# `antithesis_demo` tag gates the demo code; the `test` tag is needed for the auditor
# mock registry the harness uses.
#
# Run from the repo root (or anywhere — REPO_ROOT is resolved relative to this file)
# before `docker compose build` / `snouty launch`.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

cd "${REPO_ROOT}"
echo "Building rotation-demo (CGO_ENABLED=1, tags=antithesis_demo,test)..."
CGO_ENABLED=1 go build -tags "antithesis_demo test" \
  -o "${SCRIPT_DIR}/rotation-demo.bin" \
  ./pkg/logs/cmd/antithesis-rotation-demo/
echo "Built ${SCRIPT_DIR}/rotation-demo.bin"
