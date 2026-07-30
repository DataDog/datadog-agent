#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Required env vars — must be set before running this script.
# DD_API_KEY should come from vault: vault kv get secret/dd-api-key
# ---------------------------------------------------------------------------
: "${GCP_PROJECT:?GCP_PROJECT must be set}"
: "${AZURE_SUBSCRIPTION_ID:?AZURE_SUBSCRIPTION_ID must be set}"
: "${DD_API_KEY:?DD_API_KEY must be set (use vault: vault kv get ...)}"

# ---------------------------------------------------------------------------
# Defaults (override as needed)
# ---------------------------------------------------------------------------
DD_SITE="${DD_SITE:-datadoghq.com}"
GCP_REGION="${GCP_REGION:-us-central1}"
AZURE_REGION="${AZURE_REGION:-eastus}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

REGISTRY="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT}/serverless-test"
APP_IMAGE="${REGISTRY}/test-app:${IMAGE_TAG}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log() {
  echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"
}

# ---------------------------------------------------------------------------
# GCP: Cloud Run Service
# ---------------------------------------------------------------------------
deploy_cloudrun_service() {
  log "=== Deploying Cloud Run Service ==="

  gcloud run deploy dd-test-cloudrun-service \
    --project="${GCP_PROJECT}" \
    --region="${GCP_REGION}" \
    --image="${APP_IMAGE}" \
    --platform=managed \
    --allow-unauthenticated \
    --set-env-vars="DD_SITE=${DD_SITE},DD_ENV=dev,DD_SERVICE=dd-test-cloudrun-service,DD_VERSION=0.1.0,DD_SERVERLESS_DIAGNOSTIC_INFO=true,DD_LOGS_ENABLED=true,DD_API_KEY=${DD_API_KEY}" \
    --port=8080

  CLOUDRUN_SERVICE_URL=$(gcloud run services describe dd-test-cloudrun-service \
    --project="${GCP_PROJECT}" \
    --region="${GCP_REGION}" \
    --format="value(status.url)")

  log "Cloud Run Service URL: ${CLOUDRUN_SERVICE_URL}"
  export CLOUDRUN_SERVICE_URL
}

# ---------------------------------------------------------------------------
# GCP: Cloud Run Job
# ---------------------------------------------------------------------------
deploy_cloudrun_job() {
  log "=== Deploying Cloud Run Job ==="

  # Create or update the job
  if gcloud run jobs describe dd-test-cloudrun-job \
      --project="${GCP_PROJECT}" \
      --region="${GCP_REGION}" &>/dev/null; then
    gcloud run jobs update dd-test-cloudrun-job \
      --project="${GCP_PROJECT}" \
      --region="${GCP_REGION}" \
      --image="${APP_IMAGE}" \
      --set-env-vars="DD_SITE=${DD_SITE},DD_ENV=dev,DD_SERVICE=dd-test-cloudrun-job,DD_VERSION=0.1.0,DD_SERVERLESS_DIAGNOSTIC_INFO=true,DD_API_KEY=${DD_API_KEY}"
  else
    gcloud run jobs create dd-test-cloudrun-job \
      --project="${GCP_PROJECT}" \
      --region="${GCP_REGION}" \
      --image="${APP_IMAGE}" \
      --set-env-vars="DD_SITE=${DD_SITE},DD_ENV=dev,DD_SERVICE=dd-test-cloudrun-job,DD_VERSION=0.1.0,DD_SERVERLESS_DIAGNOSTIC_INFO=true,DD_API_KEY=${DD_API_KEY}"
  fi

  # Execute once to capture diagnostics (Cloud Run Jobs don't serve HTTP)
  gcloud run jobs execute dd-test-cloudrun-job \
    --project="${GCP_PROJECT}" \
    --region="${GCP_REGION}" \
    --wait

  log "Cloud Run Job executed successfully."
}

# ---------------------------------------------------------------------------
# GCP: Cloud Run Function v2 (gen2)
# ---------------------------------------------------------------------------
deploy_cloudrun_function() {
  log "=== Deploying Cloud Run Function v2 ==="

  gcloud functions deploy dd-test-cloudrun-function \
    --gen2 \
    --project="${GCP_PROJECT}" \
    --region="${GCP_REGION}" \
    --runtime=python311 \
    --source="${SCRIPT_DIR}/app/function" \
    --entry-point=hello \
    --trigger-http \
    --allow-unauthenticated \
    --set-env-vars="DD_SITE=${DD_SITE},DD_ENV=dev,DD_SERVICE=dd-test-cloudrun-function,DD_VERSION=0.1.0,DD_SERVERLESS_DIAGNOSTIC_INFO=true,DD_API_KEY=${DD_API_KEY}"

  CLOUDRUN_FUNCTION_URL=$(gcloud functions describe dd-test-cloudrun-function \
    --gen2 \
    --project="${GCP_PROJECT}" \
    --region="${GCP_REGION}" \
    --format="value(serviceConfig.uri)")

  log "Cloud Run Function URL: ${CLOUDRUN_FUNCTION_URL}"
  export CLOUDRUN_FUNCTION_URL
}

# ---------------------------------------------------------------------------
# Azure: Container App
# ---------------------------------------------------------------------------
deploy_container_app() {
  log "=== Deploying Azure Container App ==="

  # Create resource group (idempotent)
  az group create \
    --name dd-serverless-test-aca \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" || true

  # Create Container App environment (idempotent)
  az containerapp env create \
    --name dd-serverless-env \
    --resource-group dd-serverless-test-aca \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" || true

  # Deploy or update the Container App
  if az containerapp show \
      --name dd-test-container-app \
      --resource-group dd-serverless-test-aca \
      --subscription "${AZURE_SUBSCRIPTION_ID}" &>/dev/null; then
    az containerapp update \
      --name dd-test-container-app \
      --resource-group dd-serverless-test-aca \
      --image "${APP_IMAGE}" \
      --set-env-vars "DD_SITE=${DD_SITE}" "DD_ENV=dev" "DD_SERVICE=dd-test-container-app" "DD_VERSION=0.1.0" "DD_SERVERLESS_DIAGNOSTIC_INFO=true" "DD_LOGS_ENABLED=true" "DD_API_KEY=${DD_API_KEY}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}"
  else
    az containerapp create \
      --name dd-test-container-app \
      --resource-group dd-serverless-test-aca \
      --environment dd-serverless-env \
      --image "${APP_IMAGE}" \
      --target-port 8080 \
      --ingress external \
      --env-vars "DD_SITE=${DD_SITE}" "DD_ENV=dev" "DD_SERVICE=dd-test-container-app" "DD_VERSION=0.1.0" "DD_SERVERLESS_DIAGNOSTIC_INFO=true" "DD_LOGS_ENABLED=true" "DD_API_KEY=${DD_API_KEY}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}"
  fi

  CONTAINER_APP_URL=$(az containerapp show \
    --name dd-test-container-app \
    --resource-group dd-serverless-test-aca \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "properties.configuration.ingress.fqdn" \
    --output tsv)

  log "Azure Container App URL: https://${CONTAINER_APP_URL}"
  export CONTAINER_APP_URL="https://${CONTAINER_APP_URL}"
}

# ---------------------------------------------------------------------------
# Azure: Web App (Linux, Python 3.11)
# ---------------------------------------------------------------------------
deploy_web_app() {
  log "=== Deploying Azure Web App ==="

  # Create resource group (idempotent)
  az group create \
    --name dd-serverless-test-aas \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" || true

  # Create App Service plan (idempotent)
  az appservice plan create \
    --name dd-test-plan \
    --resource-group dd-serverless-test-aas \
    --sku B1 \
    --is-linux \
    --subscription "${AZURE_SUBSCRIPTION_ID}" || true

  # Create Web App
  az webapp create \
    --name dd-test-web-app \
    --resource-group dd-serverless-test-aas \
    --plan dd-test-plan \
    --runtime "PYTHON:3.11" \
    --subscription "${AZURE_SUBSCRIPTION_ID}"

  # Configure app settings (DD_API_KEY passed via env, not hardcoded)
  az webapp config appsettings set \
    --name dd-test-web-app \
    --resource-group dd-serverless-test-aas \
    --settings \
      DD_SITE="${DD_SITE}" \
      DD_ENV=dev \
      DD_SERVICE=dd-test-web-app \
      DD_VERSION=0.1.0 \
      DD_SERVERLESS_DIAGNOSTIC_INFO=true \
      DD_LOGS_ENABLED=true \
      DD_API_KEY="${DD_API_KEY}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}"

  # Set startup command to use ddtrace-run
  az webapp config set \
    --name dd-test-web-app \
    --resource-group dd-serverless-test-aas \
    --startup-file "DD_SERVERLESS_DIAGNOSTIC_INFO=true ddtrace-run python app.py" \
    --subscription "${AZURE_SUBSCRIPTION_ID}"

  # Zip and deploy app code (exclude the Cloud Function subdirectory)
  local zip_path="/tmp/dd-test-app.zip"
  (cd "${SCRIPT_DIR}/app" && zip -r "${zip_path}" . -x "function/*")

  az webapp deploy \
    --name dd-test-web-app \
    --resource-group dd-serverless-test-aas \
    --src-path "${zip_path}" \
    --type zip \
    --subscription "${AZURE_SUBSCRIPTION_ID}"

  WEB_APP_URL=$(az webapp show \
    --name dd-test-web-app \
    --resource-group dd-serverless-test-aas \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "defaultHostName" \
    --output tsv)

  log "Azure Web App URL: https://${WEB_APP_URL}"
  export WEB_APP_URL="https://${WEB_APP_URL}"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  log "Starting deployment to all 5 platforms..."
  log "GCP_PROJECT=${GCP_PROJECT}, GCP_REGION=${GCP_REGION}"
  log "AZURE_REGION=${AZURE_REGION}"
  log "APP_IMAGE=${APP_IMAGE}"

  deploy_cloudrun_service
  deploy_cloudrun_job
  deploy_cloudrun_function
  deploy_container_app
  deploy_web_app

  echo ""
  echo "=========================================="
  echo "Deployment complete. Service URLs:"
  echo "  Cloud Run Service : ${CLOUDRUN_SERVICE_URL:-N/A}"
  echo "  Cloud Run Job     : (no HTTP — ran once for diagnostics)"
  echo "  Cloud Run Function: ${CLOUDRUN_FUNCTION_URL:-N/A}"
  echo "  Container App     : ${CONTAINER_APP_URL:-N/A}"
  echo "  Web App           : ${WEB_APP_URL:-N/A}"
  echo "=========================================="
  echo ""
  echo "Next steps:"
  echo "  1. Trigger each service with a GET request (e.g. curl \${CLOUDRUN_SERVICE_URL})"
  echo "  2. Run ./check-logs.sh to inspect SERVERLESS_DIAGNOSTIC output"
}

main "$@"
