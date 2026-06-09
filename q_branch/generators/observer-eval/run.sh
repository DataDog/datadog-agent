#!/usr/bin/env bash
set -euo pipefail

# Build agent, build generator, submit to gs-flow — one command.
#
# Usage:
#   ./run.sh 059_Fortnite_34M_CCU_Service_Outage:memcached-saturation
#   ./run.sh 059_Fortnite_34M_CCU_Service_Outage:memcached-saturation --mode record-parquet
#   ./run.sh 059_Fortnite_34M_CCU_Service_Outage:memcached-saturation,703_shopify_bfcm_incident:kafka-partition-saturation

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
AGENT_REPO="$(cd "$SCRIPT_DIR/../../.." && pwd)"

GAR_REGISTRY="us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images"

EPISODES="${1:-}"
shift || true

MODE="scrappy-collect"
LOGS="false"

while [[ $# -gt 0 ]]; do
  case $1 in
    --mode) MODE="$2"; shift 2;;
    --logs) LOGS="true"; shift;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

if [ -z "$EPISODES" ]; then
  echo "Usage: ./run.sh <episode:scenario>[,episode:scenario,...] [--mode scrappy-collect|record-parquet] [--logs]"
  exit 1
fi

if [ -z "${GENSIM_REPO_PATH:-}" ] || [ ! -d "${GENSIM_REPO_PATH:-}" ]; then
  echo "ERROR: Set GENSIM_REPO_PATH to your gensim-episodes checkout."
  echo "  export GENSIM_REPO_PATH=/path/to/gensim-episodes"
  exit 1
fi

# Tag from branch + SHA
BRANCH=$(git -C "$AGENT_REPO" rev-parse --abbrev-ref HEAD | tr '/' '-')
SHA=$(git -C "$AGENT_REPO" rev-parse --short=7 HEAD)
AGENT_TAG="${BRANCH}-${SHA}"
AGENT_IMAGE="$GAR_REGISTRY/agent-dev:$AGENT_TAG"

echo "=== observer-eval: build → push → submit ==="
echo "  Episodes: $EPISODES"
echo "  Mode:     $MODE"
echo "  Agent:    $AGENT_IMAGE"
echo ""

# 1. GAR auth
echo "--- GAR auth ---"
gcloud auth configure-docker us-east1-docker.pkg.dev --quiet 2>/dev/null || {
  echo "Run: gcloud auth configure-docker us-east1-docker.pkg.dev"
  exit 1
}

# 2. Build agent image (must run from repo root)
echo "--- Building agent image ---"
cd "$AGENT_REPO"
dda env dev run -- dda inv agent.hacky-dev-image-build \
  --target-image="$AGENT_IMAGE" \
  --no-development

# 3. Push agent image (uses local gcloud creds)
echo "--- Pushing agent image ---"
docker push "$AGENT_IMAGE"

# 4. Build + push generator image (bakes in episodes)
echo "--- Building generator image ---"
"$SCRIPT_DIR/build.sh" --push

# 5. Submit to gs-flow
echo "--- Submitting to gs-flow ---"
EXTRA_FLAGS=""
[ "$LOGS" = "true" ] && EXTRA_FLAGS="--logs"
"$SCRIPT_DIR/submit.sh" --image "$AGENT_IMAGE" --episodes "$EPISODES" --mode "$MODE" $EXTRA_FLAGS
