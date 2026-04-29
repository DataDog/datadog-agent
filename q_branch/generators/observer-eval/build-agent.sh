#!/usr/bin/env bash
set -euo pipefail

# Build the agent with local changes and push to GAR for gs-flow.
#
# Usage:
#   ./build-agent.sh                    # build + push with default tag
#   ./build-agent.sh --tag my-feature   # custom tag
#   ./build-agent.sh --no-push          # build only, don't push

GAR_REGISTRY="us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images"
IMAGE_NAME="agent-dev"
TAG=""
PUSH=true

while [[ $# -gt 0 ]]; do
  case $1 in
    --tag) TAG="$2"; shift 2;;
    --no-push) PUSH=false; shift;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

# Default tag: branch name + short SHA
if [ -z "$TAG" ]; then
  AGENT_REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
  BRANCH=$(git -C "$AGENT_REPO" rev-parse --abbrev-ref HEAD | tr '/' '-')
  SHA=$(git -C "$AGENT_REPO" rev-parse --short=7 HEAD)
  TAG="${BRANCH}-${SHA}"
fi

FULL_IMAGE="$GAR_REGISTRY/$IMAGE_NAME:$TAG"

echo "Building agent image: $FULL_IMAGE"
echo ""

# Ensure GAR auth
echo "Checking GAR auth..."
gcloud auth configure-docker us-east1-docker.pkg.dev --quiet 2>/dev/null || {
  echo "Run: gcloud auth configure-docker us-east1-docker.pkg.dev"
  exit 1
}

# Build agent binary + Docker image for linux/amd64
AGENT_REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$AGENT_REPO"

dda env dev run -- dda inv agent.hacky-dev-image-build \
  --target-image="$FULL_IMAGE" \
  --arch=amd64 \
  --no-development

if [ "$PUSH" = true ]; then
  docker push "$FULL_IMAGE"
fi

echo ""
echo "Image: $FULL_IMAGE"
echo ""
echo "To submit to gs-flow:"
echo "  cd q_branch/generators/observer-eval"
echo "  ./submit.sh --image $FULL_IMAGE --episodes <episode:scenario>"
