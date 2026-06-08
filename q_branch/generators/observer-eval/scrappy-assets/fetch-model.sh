#!/usr/bin/env bash
# Fetch the 140MB Scrappy model (too large for git) from the durable GAR agent image.
set -euo pipefail
IMG="${1:-us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev:scrappy-detect-20260605-sc5fix}"
DIR="$(cd "$(dirname "$0")" && pwd)"
DST="$DIR/model.scrappy"
if [ -f "$DST" ]; then echo "model.scrappy already present ($DST)"; exit 0; fi
DOCKER="docker"; docker --context colima info >/dev/null 2>&1 && DOCKER="docker --context colima"
echo "Pulling $IMG ..."; $DOCKER pull --platform linux/amd64 "$IMG"
CID=$($DOCKER create --platform linux/amd64 "$IMG")
trap '$DOCKER rm -f "$CID" >/dev/null 2>&1 || true' EXIT
$DOCKER cp "$CID:/opt/scrappy/model.scrappy" "$DST"
echo "Fetched model.scrappy -> $DST ($(wc -c < "$DST") bytes; expect 139883748)"
