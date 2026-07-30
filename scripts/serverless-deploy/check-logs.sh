#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Checks all 5 deployed services for SERVERLESS_DIAGNOSTIC log output.
# Run after deploy.sh and after triggering at least one request to each service.
# ---------------------------------------------------------------------------

: "${GCP_PROJECT:?GCP_PROJECT must be set}"
: "${AZURE_SUBSCRIPTION_ID:?AZURE_SUBSCRIPTION_ID must be set}"

echo "=== GCP Cloud Run Service ==="
gcloud logging read \
  "resource.type=cloud_run_revision AND resource.labels.service_name=dd-test-cloudrun-service AND textPayload:SERVERLESS_DIAGNOSTIC" \
  --project="${GCP_PROJECT}" \
  --limit=50 \
  --format="value(textPayload)"

echo ""
echo "=== GCP Cloud Run Job ==="
gcloud logging read \
  "resource.type=cloud_run_job AND resource.labels.job_name=dd-test-cloudrun-job AND textPayload:SERVERLESS_DIAGNOSTIC" \
  --project="${GCP_PROJECT}" \
  --limit=50 \
  --format="value(textPayload)"

echo ""
echo "=== GCP Cloud Run Function ==="
gcloud logging read \
  "resource.type=cloud_run_revision AND resource.labels.function_name=dd-test-cloudrun-function AND textPayload:SERVERLESS_DIAGNOSTIC" \
  --project="${GCP_PROJECT}" \
  --limit=50 \
  --format="value(textPayload)"

echo ""
echo "=== Azure Container App ==="
az containerapp logs show \
  --name dd-test-container-app \
  --resource-group dd-serverless-test-aca \
  --subscription "${AZURE_SUBSCRIPTION_ID}" \
  --tail 100 2>/dev/null | grep SERVERLESS_DIAGNOSTIC || echo "(no diagnostic logs yet)"

echo ""
echo "=== Azure Web App ==="
# Stream logs briefly (background job) then kill
az webapp log tail \
  --name dd-test-web-app \
  --resource-group dd-serverless-test-aas \
  --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null &
TAIL_PID=$!
sleep 5
kill "${TAIL_PID}" 2>/dev/null || true
wait "${TAIL_PID}" 2>/dev/null || true
