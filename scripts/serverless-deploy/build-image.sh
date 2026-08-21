#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Builds the custom serverless-init image (from the current branch) and the
# test-app image, then pushes both to GCR Artifact Registry.
# ---------------------------------------------------------------------------

: "${GCP_PROJECT:?GCP_PROJECT must be set}"

GCP_REGION="${GCP_REGION:-us-central1}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
AZURE_REGION="${AZURE_REGION:-eastus}"
AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID:-}"
AZURE_RESOURCE_GROUP="${AZURE_RESOURCE_GROUP:-dd-serverless-test-aca}"
ACR_NAME="${ACR_NAME:-ddsvlstestaca}"

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
GIT_COMMIT="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
AGENT_VERSION="$(python3 -c "import json; d=json.load(open('${REPO_ROOT}/release.json')); print(d['current_milestone'])" 2>/dev/null || echo dev)-dev"
log "Building serverless-init image from branch source (commit=${GIT_COMMIT}, agent=${AGENT_VERSION})..."
docker build \
  --platform linux/amd64 \
  --build-arg GIT_COMMIT="${GIT_COMMIT}" \
  --build-arg AGENT_VERSION="${AGENT_VERSION}" \
  -f "${SCRIPT_DIR}/Dockerfile.serverless-init" \
  -t "${REGISTRY}/serverless-init:${IMAGE_TAG}" \
  "${REPO_ROOT}"

# ---------------------------------------------------------------------------
# Build the test app image (wrapper: serverless-init as entrypoint)
# ---------------------------------------------------------------------------
log "Building test-app image (embedding serverless-init from branch)..."
docker build \
  --platform linux/amd64 \
  --build-arg REGISTRY="${REGISTRY}" \
  --build-arg IMAGE_TAG="${IMAGE_TAG}" \
  -f "${SCRIPT_DIR}/app/Dockerfile" \
  -t "${REGISTRY}/test-app:${IMAGE_TAG}" \
  "${SCRIPT_DIR}/app"

# ---------------------------------------------------------------------------
# Build the plain app image (sidecar: no serverless-init, just the Python app)
# ---------------------------------------------------------------------------
log "Building plain-app image (for sidecar deployments)..."
docker build \
  --platform linux/amd64 \
  -f "${SCRIPT_DIR}/app/Dockerfile.plain" \
  -t "${REGISTRY}/plain-app:${IMAGE_TAG}" \
  "${SCRIPT_DIR}/app"

# ---------------------------------------------------------------------------
# Push both images
# ---------------------------------------------------------------------------
log "Pushing serverless-init image..."
docker push "${REGISTRY}/serverless-init:${IMAGE_TAG}"

log "Pushing test-app image..."
docker push "${REGISTRY}/test-app:${IMAGE_TAG}"

log "Pushing plain-app image..."
docker push "${REGISTRY}/plain-app:${IMAGE_TAG}"

log "Done. GAR images pushed:"
log "  ${REGISTRY}/serverless-init:${IMAGE_TAG}"
log "  ${REGISTRY}/test-app:${IMAGE_TAG}"
log "  ${REGISTRY}/plain-app:${IMAGE_TAG}"

# ---------------------------------------------------------------------------
# Optional: also push to Azure Container Registry
# ---------------------------------------------------------------------------
if [[ -n "${AZURE_SUBSCRIPTION_ID}" ]]; then
  ACR_REGISTRY="${ACR_NAME}.azurecr.io"
  log "Pushing to Azure Container Registry (${ACR_REGISTRY})..."

  # Create resource group and ACR (both idempotent)
  az group create \
    --name "${AZURE_RESOURCE_GROUP}" \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  az acr create \
    --name "${ACR_NAME}" \
    --resource-group "${AZURE_RESOURCE_GROUP}" \
    --sku Basic \
    --location "${AZURE_REGION}" \
    --admin-enabled true \
    --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null || true

  az acr login --name "${ACR_NAME}" --subscription "${AZURE_SUBSCRIPTION_ID}"

  docker tag "${REGISTRY}/serverless-init:${IMAGE_TAG}" "${ACR_REGISTRY}/serverless-init:${IMAGE_TAG}"
  docker push "${ACR_REGISTRY}/serverless-init:${IMAGE_TAG}"

  docker tag "${REGISTRY}/test-app:${IMAGE_TAG}" "${ACR_REGISTRY}/test-app:${IMAGE_TAG}"
  docker push "${ACR_REGISTRY}/test-app:${IMAGE_TAG}"

  docker tag "${REGISTRY}/plain-app:${IMAGE_TAG}" "${ACR_REGISTRY}/plain-app:${IMAGE_TAG}"
  docker push "${ACR_REGISTRY}/plain-app:${IMAGE_TAG}"

  log "ACR images pushed:"
  log "  ${ACR_REGISTRY}/serverless-init:${IMAGE_TAG}"
  log "  ${ACR_REGISTRY}/test-app:${IMAGE_TAG}"
  log "  ${ACR_REGISTRY}/plain-app:${IMAGE_TAG}"
fi
