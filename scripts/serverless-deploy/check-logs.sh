#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Collects SERVERLESS_DIAGNOSTIC logs from all 8 deployed services and writes
# the full output to a dated txt file. Generates a REDAPL SQL query at the end
# using the agent UUIDs extracted from the diagnostic blocks.
#
# Usage: GCP_PROJECT=... AZURE_SUBSCRIPTION_ID=... ./check-logs.sh
# Output file: diagnostic-results-YYYYMMDD-HHMMSS.txt (or set OUTPUT_FILE)
# ---------------------------------------------------------------------------

: "${GCP_PROJECT:?GCP_PROJECT must be set}"
AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID:-}"
AZURE_RG_ACA="${AZURE_RG_ACA:-dd-serverless-test-aca}"
AZURE_RG_AAS="${AZURE_RG_AAS:-dd-serverless-test-aas}"

OUTPUT_FILE="${OUTPUT_FILE:-${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}/diagnostic-results-$(date -u +%Y%m%d-%H%M%S).txt}"

log() { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"; }

# ---------------------------------------------------------------------------
# Log fetchers
# ---------------------------------------------------------------------------

_gcp_logs() {
  local label="$1" resource_type="$2" filter_key="$3" filter_val="$4"
  echo ""
  echo "================================================================"
  echo "# ${label}"
  echo "================================================================"
  gcloud logging read \
    "resource.type=${resource_type} AND resource.labels.${filter_key}=${filter_val} AND textPayload:SERVERLESS_DIAGNOSTIC" \
    --project="${GCP_PROJECT}" \
    --limit=100 \
    --format="value(textPayload)" \
  || echo "(no diagnostic logs yet — trigger a request and retry)"
}

_azure_containerapp_logs() {
  local label="$1" app_name="$2" container="${3:-}"
  [[ -z "${AZURE_SUBSCRIPTION_ID}" ]] && return
  echo ""
  echo "================================================================"
  echo "# ${label}"
  echo "================================================================"
  local args=(--name "${app_name}" --resource-group "${AZURE_RG_ACA}" \
               --subscription "${AZURE_SUBSCRIPTION_ID}" --tail 300)
  [[ -n "${container}" ]] && args+=(--container "${container}")
  az containerapp logs show "${args[@]}" 2>/dev/null \
  | grep "SERVERLESS_DIAGNOSTIC" \
  || echo "(no diagnostic logs yet — trigger a request and retry)"
}

_azure_webapp_logs() {
  local label="$1" app_name="$2" rg="${3:-${AZURE_RG_AAS}}"
  [[ -z "${AZURE_SUBSCRIPTION_ID}" ]] && return
  echo ""
  echo "================================================================"
  echo "# ${label}"
  echo "================================================================"
  # az webapp log tail only shows live streaming (misses already-warm containers).
  # Use log download + grep instead — this captures archived logs from the
  # containerStream and default_docker files written since last startup.
  local tmp_zip
  tmp_zip=$(mktemp /tmp/webapp-logs-XXXXXX.zip)
  az webapp log download \
    --name "${app_name}" \
    --resource-group "${rg}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --log-file "${tmp_zip}" 2>/dev/null \
  && unzip -p "${tmp_zip}" 2>/dev/null \
     | grep -a "SERVERLESS_DIAGNOSTIC" \
     | sort -u \
  || echo "(no diagnostic logs yet — trigger a request and retry)"
  rm -f "${tmp_zip}"
}

# ---------------------------------------------------------------------------
# Run all checks
# ---------------------------------------------------------------------------
run_checks() {
  echo "================================================================"
  echo "SVLS-9526 — Serverless Init Diagnostic Results"
  echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  echo "================================================================"

  # GCP
  _gcp_logs "#1 GCP Cloud Run Service (init-container)" \
    cloud_run_revision service_name nina-cloudrun-init

  _gcp_logs "#2 GCP Cloud Run Service (sidecar)" \
    cloud_run_revision service_name nina-cloudrun-sidecar

  _gcp_logs "#3 GCP Cloud Run Job (init-container)" \
    cloud_run_job job_name nina-cloudrun-job

  _gcp_logs "#4 GCP Cloud Run Function gen2 (sidecar)" \
    cloud_run_revision service_name nina-cloudrun-function-sidecar

  # Azure Container Apps
  _azure_containerapp_logs "#5 Azure Container App (init-container)" \
    nina-containerapp-init

  _azure_containerapp_logs "#6 Azure Container App (sidecar, dd-agent)" \
    nina-containerapp-sidecar dd-agent

  # Azure Web Apps
  _azure_webapp_logs "#7 Azure Web App Linux Containers (init-container)" \
    nina-webapp-container

  _azure_webapp_logs "#8 Azure Web App SITECONTAINERS (sidecar, dd-agent)" \
    nina-webapp-sidecar

  _azure_webapp_logs "#9 Azure Web App Linux Code + sidecar (dd-agent)" \
    nina-webapp-linux-code
}

# ---------------------------------------------------------------------------
# Extract agent UUIDs from the collected output and generate the REDAPL query
# ---------------------------------------------------------------------------
generate_sql() {
  local output_file="$1"

  # Extract UUIDs from lines like: [SERVERLESS_DIAGNOSTIC] uuid (agent): <uuid>
  # Use python3 for portable regex (BSD grep lacks -P, GNU grep has it but not on macOS).
  local uuids
  uuids=$(python3 -c "
import re, sys
text = open('${output_file}').read()
uuids = re.findall(r'uuid \(agent\):\s+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})', text)
print('\n'.join(sorted(set(uuids))))
" 2>/dev/null || true)

  echo ""
  echo "================================================================"
  echo "# REDAPL SQL Query — datadog_agent table"
  echo "# Run in: go/redapl → Queries → SQL"
  echo "================================================================"
  echo ""

  if [[ -n "${uuids}" ]]; then
    echo "-- Query by agent UUID (extracted from diagnostic logs above):"
    echo "SELECT"
    echo "    _key             AS uuid,"
    echo "    agent_version,"
    echo "    first_seen_at,"
    echo "    api_key_uuid"
    echo "FROM datadog_agent"
    echo "WHERE _key IN ("
    while IFS= read -r uuid; do
      echo "    '${uuid}',"
    done <<< "${uuids}"
    echo ")"
    echo "ORDER BY first_seen_at DESC;"
    echo ""
    echo "-- Alternative: query by agent version + time window (catches any UUID):"
  else
    echo "-- No UUIDs extracted yet (run after services have started)."
    echo "-- Fallback query by agent version + time window:"
  fi

  echo "SELECT"
  echo "    _key             AS uuid,"
  echo "    agent_version,"
  echo "    first_seen_at,"
  echo "    api_key_uuid"
  echo "FROM datadog_agent"
  echo "WHERE first_seen_at > NOW() - INTERVAL 2 HOUR"
  echo "  AND agent_version LIKE '7.%'"
  echo "ORDER BY first_seen_at DESC"
  echo "LIMIT 20;"
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
log "Collecting diagnostic logs → ${OUTPUT_FILE}"

run_checks | tee "${OUTPUT_FILE}"
generate_sql "${OUTPUT_FILE}" | tee -a "${OUTPUT_FILE}"

echo ""
log "Done. Full output written to: ${OUTPUT_FILE}"
log "Agent UUIDs in output:"
python3 -c "
import re
text = open('${OUTPUT_FILE}').read()
uuids = sorted(set(re.findall(r'uuid \(agent\):\s+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})', text)))
print('\n'.join('  ' + u for u in uuids))
" 2>/dev/null || true
