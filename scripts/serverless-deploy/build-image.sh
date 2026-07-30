#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Builds the custom serverless-init image (from the current branch) and the
# test-app image, then pushes both to GCR Artifact Registry.
# ---------------------------------------------------------------------------

: "${GCP_PROJECT:?GCP_PROJECT must be set}"

GCP_REGION="${GCP_REGION:-us-central1}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

REGISTRY="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT}/serverless-test"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Repo root is two levels above scripts/serverless-deploy/
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

log() {
  echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"
}

# ---------------------------------------------------------------------------
# Configure Docker to push to GCR Artifact Registry
# ---------------------------------------------------------------------------
gcloud auth configure-docker "${GCP_REGION}-docker.pkg.dev" --quiet

# ---------------------------------------------------------------------------
# Create Artifact Registry repository (idempotent)
# ---------------------------------------------------------------------------
log "Ensuring Artifact Registry repository exists..."
gcloud artifacts repositories create serverless-test \
  --repository-format=docker \
  --location="${GCP_REGION}" \
  --project="${GCP_PROJECT}" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Build serverless-init from current branch source
# ---------------------------------------------------------------------------
log "Building serverless-init image from branch source..."
docker build \
  --platform linux/amd64 \
  -f "${SCRIPT_DIR}/Dockerfile.serverless-init" \
  -t "${REGISTRY}/serverless-init:${IMAGE_TAG}" \
  "${REPO_ROOT}"

# ---------------------------------------------------------------------------
# Build the test app image
# ---------------------------------------------------------------------------
log "Building test-app image..."
docker build \
  --platform linux/amd64 \
  -f "${SCRIPT_DIR}/app/Dockerfile" \
  -t "${REGISTRY}/test-app:${IMAGE_TAG}" \
  "${SCRIPT_DIR}/app"

# ---------------------------------------------------------------------------
# Push both images
# ---------------------------------------------------------------------------
log "Pushing serverless-init image..."
docker push "${REGISTRY}/serverless-init:${IMAGE_TAG}"

log "Pushing test-app image..."
docker push "${REGISTRY}/test-app:${IMAGE_TAG}"

log "Done. Images pushed:"
log "  ${REGISTRY}/serverless-init:${IMAGE_TAG}"
log "  ${REGISTRY}/test-app:${IMAGE_TAG}"
