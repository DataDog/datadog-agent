#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# demo.sh — SVLS-9526 end-to-end POC
#
# Builds serverless-init from the current branch, deploys all 9 test services
# across GCP and Azure, triggers cold starts, waits for the inventory runner
# to fire, then prints the REDAPL SQL query to validate that agents appear in
# the datadog_agent table.
#
# Related PRs:
#   #54537 — ForceCollect(): send inventory payload on every cold start
#   #54538 — inventories_first_run_delay config key
#   #54543 — cmd/serverless-init/inventory: serverless_* fields in payload
#
# Usage:
#   export GCP_PROJECT=datadog-serverless-gcp-demo
#   export AZURE_SUBSCRIPTION_ID=<your-sub-id>
#   export DD_API_KEY=$(vault kv get -field=api_key kv/dd/api_keys/dddev)
#   export IMAGE_TAG=1.10.2-poc-$(date +%Y%m%d)   # optional, defaults to 'poc'
#   ./demo.sh
#
# To skip the build (use an already-pushed image):
#   SKIP_BUILD=true ./demo.sh
#
# To skip deployment (only trigger + collect logs):
#   SKIP_DEPLOY=true ./demo.sh
# ---------------------------------------------------------------------------

: "${GCP_PROJECT:?GCP_PROJECT must be set (e.g. datadog-serverless-gcp-demo)}"
: "${DD_API_KEY:?DD_API_KEY must be set — vault kv get -field=api_key kv/dd/api_keys/dddev}"
: "${AZURE_SUBSCRIPTION_ID:?AZURE_SUBSCRIPTION_ID must be set}"

IMAGE_TAG="${IMAGE_TAG:-poc}"
SKIP_BUILD="${SKIP_BUILD:-false}"
SKIP_DEPLOY="${SKIP_DEPLOY:-false}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

log() { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"; }
header() {
  echo ""
  echo "=================================================="
  echo "  $*"
  echo "=================================================="
}

# ---------------------------------------------------------------------------
# Step 1 — Build and push images
# ---------------------------------------------------------------------------
if [[ "${SKIP_BUILD}" != "true" ]]; then
  header "Step 1/3 — Build + push images (IMAGE_TAG=${IMAGE_TAG})"
  GCP_PROJECT="${GCP_PROJECT}" \
  AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
  IMAGE_TAG="${IMAGE_TAG}" \
    "${SCRIPT_DIR}/build-image.sh"
else
  log "Skipping build (SKIP_BUILD=true)"
fi

# ---------------------------------------------------------------------------
# Step 2 — Deploy all 9 services
# ---------------------------------------------------------------------------
if [[ "${SKIP_DEPLOY}" != "true" ]]; then
  header "Step 2/3 — Deploy all 9 services"
  GCP_PROJECT="${GCP_PROJECT}" \
  DD_API_KEY="${DD_API_KEY}" \
  AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
  IMAGE_TAG="${IMAGE_TAG}" \
    "${SCRIPT_DIR}/deploy.sh"
else
  log "Skipping deploy (SKIP_DEPLOY=true)"
fi

# ---------------------------------------------------------------------------
# Step 3 — Trigger cold starts + collect diagnostic logs + print REDAPL query
# ---------------------------------------------------------------------------
header "Step 3/3 — Trigger cold starts + collect logs"
log "Forcing a cold start on each service to exercise the ForceCollect() path..."
log "Waiting ~75s after triggers for the inventory runner to fire and send the payload."
log ""

GCP_PROJECT="${GCP_PROJECT}" \
AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
IMAGE_TAG="${IMAGE_TAG}" \
  "${SCRIPT_DIR}/deploy.sh" trigger_all

# ---------------------------------------------------------------------------
# Done — print REDAPL validation instructions
# ---------------------------------------------------------------------------
echo ""
echo "=================================================="
echo "  POC complete — validate in REDAPL"
echo "=================================================="
echo ""
echo "The REDAPL SQL query above was extracted from diagnostic logs."
echo "To run it:"
echo ""
echo "  1. Go to go/redapl → Queries → SQL"
echo "  2. Paste the query printed above and run it"
echo "  3. You should see one row per service (GCP) or per App Service Plan"
echo "     VM instance (Azure) that successfully sent an inventory payload."
echo ""
echo "What to look for:"
echo "  - GCP Cloud Run:  new UUID per cold start (ForceCollect fires before scale-to-zero)"
echo "  - Azure ACA:      stable UUID (same container runtime across restarts)"
echo "  - Azure AAS:      stable DMI UUID per underlying VM; apps on the same"
echo "                    App Service Plan share a UUID (CCRID needed for uniqueness)"
echo ""
echo "These results demonstrate the gap that motivates serverless_init_agent"
echo "(a separate REDAPL table keyed by CCRID instead of UUID)."
echo ""
echo "Related PRs:"
echo "  #54537  ForceCollect() — send inventory payload on every cold start"
echo "  #54538  inventories_first_run_delay config key"
echo "  #54543  cmd/serverless-init/inventory: serverless_* fields in payload"
