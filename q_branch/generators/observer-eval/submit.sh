#!/usr/bin/env bash
set -euo pipefail

# Build the generator image and submit an observer-eval job to gs-flow.
#
# Usage:
#   ./submit.sh --image <agent-image> --episodes "059_Fortnite:memcached-saturation"
#   ./submit.sh --image ... --episodes "..." --mode scrappy-collect
#
# Requires:
#   GENSIM_REPO_PATH - path to gensim-episodes checkout
#   GS_FLOW_URL      - gs-flow API endpoint (default: https://gs-flow.us1.staging.dog)
#   DD_API_KEY       - Datadog API key
#   DD_APP_KEY       - Datadog app key

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GS_FLOW_URL="${GS_FLOW_URL:-https://gs-flow.us1.staging.dog}"
GAR_REGISTRY="us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images"
GENERATOR_IMAGE="$GAR_REGISTRY/observer-eval:latest"

# Parse args
IMAGE="${GAR_REGISTRY}/agent-dev:scrappy-collect-amd64"
EPISODES=""
MODE="scrappy-collect"
LOGS_ENABLED="false"
SKIP_BUILD=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --image) IMAGE="$2"; shift 2;;
    --episodes) EPISODES="$2"; shift 2;;
    --mode) MODE="$2"; shift 2;;
    --logs) LOGS_ENABLED="true"; shift;;
    --skip-build) SKIP_BUILD=true; shift;;
    --generator-image) GENERATOR_IMAGE="$2"; shift 2;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

if [ -z "$IMAGE" ] || [ -z "$EPISODES" ]; then
  echo "Usage: $0 --image <agent-image> --episodes <ep1:scen1,ep2:scen2> [--mode scrappy-collect|record-parquet] [--logs]"
  exit 1
fi

# 1. Build + push generator image (linux/amd64, bakes in only the needed episodes)
if [ "$SKIP_BUILD" = false ]; then
  echo "--- Building generator image ---"
  "$SCRIPT_DIR/build.sh" --push --episodes "$EPISODES"
else
  echo "--- Skipping generator build ---"
fi

# 2. Submit to gs-flow
echo ""
echo "Submitting to gs-flow..."
echo "  Generator: $GENERATOR_IMAGE"
echo "  Agent:     $IMAGE"
echo "  Episodes:  $EPISODES"
echo "  Mode:      $MODE"

RESPONSE=$(curl -s -X POST "$GS_FLOW_URL/api/v1/jobs" \
  -H "Content-Type: application/json" \
  -d "{
    \"backend\": \"observer-eval\",
    \"generator_image\": \"$GENERATOR_IMAGE\",
    \"secrets\": {
      \"AGENT_IMAGE\": \"$IMAGE\",
      \"EPISODES\": \"$EPISODES\",
      \"GENSIM_MODE\": \"$MODE\",
      \"LOGS_ENABLED\": \"$LOGS_ENABLED\",
      \"DD_API_KEY\": \"$DD_API_KEY\",
      \"DD_APP_KEY\": \"$DD_APP_KEY\",
      \"DD_SITE\": \"${DD_SITE:-datadoghq.com}\",
      \"GAR_REGISTRY\": \"$GAR_REGISTRY\"
    }
  }")

JOB_ID=$(echo "$RESPONSE" | jq -r '.job_id // .id // empty')

if [ -n "$JOB_ID" ]; then
  echo ""
  echo "Job submitted: $JOB_ID"
  echo ""
  echo "Check status:"
  echo "  curl -s $GS_FLOW_URL/api/v1/jobs/$JOB_ID | jq"
  echo ""
  echo "Fetch artifacts when done:"
  echo "  curl -s $GS_FLOW_URL/api/v1/jobs/$JOB_ID/artifacts -o artifacts.tar.gz"
else
  echo "Submit failed:"
  echo "$RESPONSE" | jq . 2>/dev/null || echo "$RESPONSE"
  exit 1
fi
