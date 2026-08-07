#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Required env vars — must be set before running this script.
# DD_API_KEY should come from vault: vault kv get secret/dd-api-key
# ---------------------------------------------------------------------------
: "${GCP_PROJECT:?GCP_PROJECT must be set}"
: "${DD_API_KEY:?DD_API_KEY must be set (use vault: vault kv get ...)}"
: "${AZURE_SUBSCRIPTION_ID:?AZURE_SUBSCRIPTION_ID must be set}"

# ---------------------------------------------------------------------------
# Defaults (override as needed)
# ---------------------------------------------------------------------------
DD_SITE="${DD_SITE:-datadoghq.com}"
GCP_REGION="${GCP_REGION:-us-central1}"
AZURE_REGION="${AZURE_REGION:-eastus}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
ACR_NAME="${ACR_NAME:-ddsvlstestaca}"
AZURE_RESOURCE_GROUP_ACA="${AZURE_RESOURCE_GROUP_ACA:-dd-serverless-test-aca}"
AZURE_RESOURCE_GROUP_AAS="${AZURE_RESOURCE_GROUP_AAS:-dd-serverless-test-aas}"
AZURE_CONTAINER_ENV="${AZURE_CONTAINER_ENV:-dd-serverless-env}"

REGISTRY="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT}/serverless-test"
GCP_REGISTRY="${REGISTRY}"
APP_IMAGE="${REGISTRY}/test-app:${IMAGE_TAG}"
ACR_REGISTRY="${ACR_NAME}.azurecr.io"
AZURE_APP_IMAGE="${ACR_REGISTRY}/test-app:${IMAGE_TAG}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

log() { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"; }

# ---------------------------------------------------------------------------
# ACR credential helper (called by Azure functions that need registry auth)
# ---------------------------------------------------------------------------
_acr_creds() {
  az acr update --name "${ACR_NAME}" --admin-enabled true \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
  ACR_USER=$(az acr credential show --name "${ACR_NAME}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" --query "username" -o tsv)
  ACR_PASS=$(az acr credential show --name "${ACR_NAME}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" --query "passwords[0].value" -o tsv)
}

# ---------------------------------------------------------------------------
# #1 — GCP Cloud Run Service (init-container)
# ---------------------------------------------------------------------------
deploy_cloudrun_service() {
  log "=== #1 GCP Cloud Run Service (init-container) ==="

  gcloud run deploy nina-cloudrun-init \
    --project="${GCP_PROJECT}" \
    --region="${GCP_REGION}" \
    --image="${APP_IMAGE}" \
    --platform=managed \
    --allow-unauthenticated \
    --set-env-vars="DD_SITE=${DD_SITE},DD_SERVICE=nina-cloudrun-init,DD_ENV=nina,DD_SERVERLESS_DIAGNOSTIC_INFO=true,DD_API_KEY=${DD_API_KEY}" \
    --port=8080

  CLOUDRUN_SERVICE_URL=$(gcloud run services describe nina-cloudrun-init \
    --project="${GCP_PROJECT}" --region="${GCP_REGION}" \
    --format="value(status.url)")
  log "#1 URL: ${CLOUDRUN_SERVICE_URL}"
  export CLOUDRUN_SERVICE_URL
}

# ---------------------------------------------------------------------------
# #2 — GCP Cloud Run Service (sidecar, multi-container via knative YAML)
# ---------------------------------------------------------------------------
deploy_cloudrun_sidecar() {
  log "=== #2 GCP Cloud Run Service (sidecar) ==="

  local tmp_yaml="/tmp/cloudrun-sidecar.yaml"
  GCP_PROJECT="${GCP_PROJECT}" GCP_REGISTRY="${GCP_REGISTRY}" \
  IMAGE_TAG="${IMAGE_TAG}" DD_SITE="${DD_SITE}" DD_API_KEY="${DD_API_KEY}" \
    envsubst < "${SCRIPT_DIR}/app/cloudrun-sidecar.yaml" > "${tmp_yaml}"

  gcloud run services replace "${tmp_yaml}" \
    --project="${GCP_PROJECT}" --region="${GCP_REGION}"

  CLOUDRUN_SIDECAR_URL=$(gcloud run services describe nina-cloudrun-sidecar \
    --project="${GCP_PROJECT}" --region="${GCP_REGION}" \
    --format="value(status.url)")
  log "#2 URL: ${CLOUDRUN_SIDECAR_URL}"
  export CLOUDRUN_SIDECAR_URL
}

# ---------------------------------------------------------------------------
# #3 — GCP Cloud Run Job (init-container, runs job.py which exits cleanly)
# ---------------------------------------------------------------------------
deploy_cloudrun_job() {
  log "=== #3 GCP Cloud Run Job (init-container) ==="

  local common_flags=(
    --project="${GCP_PROJECT}"
    --region="${GCP_REGION}"
    --image="${APP_IMAGE}"
    --args="python,/app/job.py"
    --set-env-vars="DD_SITE=${DD_SITE},DD_SERVICE=nina-cloudrun-job,DD_ENV=nina,DD_SERVERLESS_DIAGNOSTIC_INFO=true,DD_API_KEY=${DD_API_KEY}"
  )

  if gcloud run jobs describe nina-cloudrun-job \
      --project="${GCP_PROJECT}" --region="${GCP_REGION}" &>/dev/null; then
    gcloud run jobs update nina-cloudrun-job "${common_flags[@]}"
  else
    gcloud run jobs create nina-cloudrun-job "${common_flags[@]}"
  fi

  gcloud run jobs execute nina-cloudrun-job \
    --project="${GCP_PROJECT}" --region="${GCP_REGION}" --wait || true

  log "#3 Cloud Run Job executed."
}

# ---------------------------------------------------------------------------
# #4 — GCP Cloud Run Function gen2 (sidecar, deployed as Cloud Run Service)
# ---------------------------------------------------------------------------
deploy_cloudrun_function_sidecar() {
  log "=== #4 GCP Cloud Run Function gen2 (sidecar) ==="

  local tmp_yaml="/tmp/cloudrun-function-sidecar.yaml"
  GCP_PROJECT="${GCP_PROJECT}" GCP_REGISTRY="${GCP_REGISTRY}" \
  IMAGE_TAG="${IMAGE_TAG}" DD_SITE="${DD_SITE}" DD_API_KEY="${DD_API_KEY}" \
    envsubst < "${SCRIPT_DIR}/app/cloudrun-function-sidecar.yaml" > "${tmp_yaml}"

  gcloud run services replace "${tmp_yaml}" \
    --project="${GCP_PROJECT}" --region="${GCP_REGION}"

  CLOUDRUN_FUNCTION_SIDECAR_URL=$(gcloud run services describe nina-cloudrun-function-sidecar \
    --project="${GCP_PROJECT}" --region="${GCP_REGION}" \
    --format="value(status.url)")
  log "#4 URL: ${CLOUDRUN_FUNCTION_SIDECAR_URL}"
  export CLOUDRUN_FUNCTION_SIDECAR_URL
}

# ---------------------------------------------------------------------------
# #5 — Azure Container App (init-container)
# ---------------------------------------------------------------------------
deploy_container_app() {
  log "=== #5 Azure Container App (init-container) ==="

  _acr_creds

  az group create \
    --name "${AZURE_RESOURCE_GROUP_ACA}" \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  az containerapp env create \
    --name "${AZURE_CONTAINER_ENV}" \
    --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null || true

  local dd_env_vars=(
    "DD_SITE=${DD_SITE}"
    "DD_SERVICE=nina-containerapp-init"
    "DD_ENV=nina"
    "DD_SERVERLESS_DIAGNOSTIC_INFO=true"
    "DD_LOG_LEVEL=warn"
    "DD_AZURE_SUBSCRIPTION_ID=${AZURE_SUBSCRIPTION_ID}"
    "DD_AZURE_RESOURCE_GROUP=${AZURE_RESOURCE_GROUP_ACA}"
    "DD_API_KEY=${DD_API_KEY}"
  )

  if az containerapp show \
      --name "nina-containerapp-init" \
      --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" &>/dev/null; then
    az containerapp registry set \
      --name "nina-containerapp-init" \
      --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
      --server "${ACR_REGISTRY}" \
      --username "${ACR_USER}" \
      --password "${ACR_PASS}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}"
    az containerapp update \
      --name "nina-containerapp-init" \
      --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
      --image "${AZURE_APP_IMAGE}" \
      --set-env-vars "${dd_env_vars[@]}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}"
  else
    az containerapp create \
      --name "nina-containerapp-init" \
      --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
      --environment "${AZURE_CONTAINER_ENV}" \
      --image "${AZURE_APP_IMAGE}" \
      --target-port 8080 \
      --ingress external \
      --registry-server "${ACR_REGISTRY}" \
      --registry-username "${ACR_USER}" \
      --registry-password "${ACR_PASS}" \
      --env-vars "${dd_env_vars[@]}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}"
  fi

  az containerapp ingress enable \
    --name "nina-containerapp-init" \
    --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
    --type external \
    --target-port 8080 \
    --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null || true

  CONTAINER_APP_URL=$(az containerapp show \
    --name "nina-containerapp-init" \
    --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "properties.configuration.ingress.fqdn" \
    --output tsv)
  log "#5 URL: https://${CONTAINER_APP_URL}"
  export CONTAINER_APP_URL="https://${CONTAINER_APP_URL}"
}

# ---------------------------------------------------------------------------
# #6 — Azure Container App (sidecar, multi-container via az rest PUT)
# az containerapp create/update --yaml has a known Bool serialisation bug.
# ---------------------------------------------------------------------------
deploy_container_app_sidecar() {
  log "=== #6 Azure Container App (sidecar) ==="

  _acr_creds

  local app_name="nina-containerapp-sidecar"
  local env_id="/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${AZURE_RESOURCE_GROUP_ACA}/providers/Microsoft.App/managedEnvironments/${AZURE_CONTAINER_ENV}"
  local tmp_json="/tmp/containerapp-sidecar.json"

  python3 - <<PYEOF
import json
body = {
  "location": "eastus",
  "properties": {
    "managedEnvironmentId": "${env_id}",
    "configuration": {
      "ingress": {"external": True, "targetPort": 8080, "traffic": [{"latestRevision": True, "weight": 100}]},
      "registries": [{"server": "${ACR_REGISTRY}", "username": "${ACR_USER}", "passwordSecretRef": "acr-password"}],
      "secrets": [{"name": "acr-password", "value": "${ACR_PASS}"}, {"name": "dd-api-key", "value": "${DD_API_KEY}"}]
    },
    "template": {
      "containers": [
        {
          "name": "app",
          "image": "${ACR_REGISTRY}/plain-app:${IMAGE_TAG}",
          "resources": {"cpu": 0.5, "memory": "1Gi"},
          "env": [
            {"name": "DD_SERVICE", "value": "nina-containerapp-sidecar"},
            {"name": "DD_ENV",     "value": "nina"}
          ]
        },
        {
          "name": "dd-agent",
          "image": "${ACR_REGISTRY}/serverless-init:${IMAGE_TAG}",
          "resources": {"cpu": 0.5, "memory": "1Gi"},
          "env": [
            {"name": "DD_SITE",                       "value": "${DD_SITE}"},
            {"name": "DD_SERVICE",                    "value": "nina-containerapp-sidecar"},
            {"name": "DD_ENV",                        "value": "nina"},
            {"name": "DD_APM_NON_LOCAL_TRAFFIC",      "value": "true"},
            {"name": "DD_DOGSTATSD_NON_LOCAL_TRAFFIC","value": "true"},
            {"name": "DD_SERVERLESS_DIAGNOSTIC_INFO", "value": "true"},
            {"name": "DD_LOG_LEVEL",                  "value": "warn"},
            {"name": "DD_HEALTH_PORT",                "value": "5555"},
            {"name": "DD_AZURE_SUBSCRIPTION_ID",      "value": "${AZURE_SUBSCRIPTION_ID}"},
            {"name": "DD_AZURE_RESOURCE_GROUP",       "value": "${AZURE_RESOURCE_GROUP_ACA}"},
            {"name": "DD_API_KEY",                    "secretRef": "dd-api-key"}
          ]
        }
      ]
    }
  }
}
with open("${tmp_json}", "w") as f:
    json.dump(body, f)
PYEOF

  az rest \
    --method PUT \
    --url "https://management.azure.com/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${AZURE_RESOURCE_GROUP_ACA}/providers/Microsoft.App/containerApps/${app_name}?api-version=2024-03-01" \
    --body "@${tmp_json}" \
    --headers "Content-Type=application/json"

  CONTAINER_APP_SIDECAR_URL=$(az containerapp show \
    --name "${app_name}" \
    --resource-group "${AZURE_RESOURCE_GROUP_ACA}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "properties.configuration.ingress.fqdn" \
    --output tsv)
  log "#6 URL: https://${CONTAINER_APP_SIDECAR_URL}"
  export CONTAINER_APP_SIDECAR_URL="https://${CONTAINER_APP_SIDECAR_URL}"
}

# ---------------------------------------------------------------------------
# ARM helper — registers a sitecontainer (main or sidecar) via REST API.
# Ref: https://learn.microsoft.com/en-us/rest/api/appservice/web-apps/create-or-update-site-container
# ---------------------------------------------------------------------------
_webapp_sitecontainer_put() {
  local rg="$1" site="$2" name="$3" body="$4"
  az rest --method PUT \
    --url "https://management.azure.com/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${rg}/providers/Microsoft.Web/sites/${site}/sitecontainers/${name}?api-version=2024-11-01" \
    --body "${body}" >/dev/null
}

# Patches linuxFxVersion to "SITECONTAINERS" so the sidecontainers serve traffic.
# Ref: terraform-azurerm-web-app-datadog/modules/linux/main.tf (azapi_update_resource.enable_sidecar)
_webapp_enable_sitecontainers() {
  local rg="$1" site="$2"
  az rest --method PATCH \
    --url "https://management.azure.com/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${rg}/providers/Microsoft.Web/sites/${site}?api-version=2022-03-01" \
    --body '{"properties":{"siteConfig":{"linuxFxVersion":"SITECONTAINERS"}}}' >/dev/null
}

# ---------------------------------------------------------------------------
# #7 — Azure Web App Linux Containers (init-container)
# Single-container; test-app image has serverless-init as ENTRYPOINT wrapping app.
# ---------------------------------------------------------------------------
deploy_web_app_container() {
  log "=== #7 Azure Web App (Linux Containers, init-container) ==="

  _acr_creds
  local rg="${AZURE_RESOURCE_GROUP_AAS}"

  az group create \
    --name "${rg}" \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null || true

  az appservice plan create \
    --name dd-test-plan-container \
    --resource-group "${rg}" \
    --sku B2 \
    --is-linux \
    --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null || true

  if az webapp show \
      --name nina-webapp-container \
      --resource-group "${rg}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" &>/dev/null; then
    az webapp config container set \
      --name nina-webapp-container \
      --resource-group "${rg}" \
      --docker-custom-image-name "${AZURE_APP_IMAGE}" \
      --docker-registry-server-url "https://${ACR_REGISTRY}" \
      --docker-registry-server-user "${ACR_USER}" \
      --docker-registry-server-password "${ACR_PASS}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
  else
    az webapp create \
      --name nina-webapp-container \
      --resource-group "${rg}" \
      --plan dd-test-plan-container \
      --deployment-container-image-name "${AZURE_APP_IMAGE}" \
      --https-only true \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
    az webapp config container set \
      --name nina-webapp-container \
      --resource-group "${rg}" \
      --docker-registry-server-url "https://${ACR_REGISTRY}" \
      --docker-registry-server-user "${ACR_USER}" \
      --docker-registry-server-password "${ACR_PASS}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
  fi

  az webapp config appsettings set \
    --name nina-webapp-container \
    --resource-group "${rg}" \
    --settings \
      DD_SITE="${DD_SITE}" \
      DD_SERVICE=nina-webapp-container \
      DD_ENV=nina \
      DD_SERVERLESS_DIAGNOSTIC_INFO=true \
      DD_AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
      DD_AZURE_RESOURCE_GROUP="${rg}" \
      DD_API_KEY="${DD_API_KEY}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  WEB_APP_CONTAINER_URL=$(az webapp show \
    --name nina-webapp-container \
    --resource-group "${rg}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "defaultHostName" \
    --output tsv)
  log "#7 URL: https://${WEB_APP_CONTAINER_URL}"
  export WEB_APP_CONTAINER_URL="https://${WEB_APP_CONTAINER_URL}"
}

# ---------------------------------------------------------------------------
# #8 — Azure Web App Linux Containers (SITECONTAINERS sidecar)
# main container: plain-app (port 8080) as isMain=true sitecontainer
# dd-agent sidecar: serverless-init with no args → sidecar mode
#
# Env vars go in App Settings (az webapp config appsettings set); the dd-agent
# sidecontainer inherits them automatically. Do NOT put env vars in the
# sitecontainer body — they do not survive webapp restarts.
# ---------------------------------------------------------------------------
deploy_web_app_sidecar() {
  log "=== #8 Azure Web App (SITECONTAINERS, sidecar) ==="

  _acr_creds
  local rg="${AZURE_RESOURCE_GROUP_AAS}"

  az group create \
    --name "${rg}" \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null || true

  az appservice plan create \
    --name dd-test-plan-sidecar \
    --resource-group "${rg}" \
    --sku B2 \
    --is-linux \
    --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null || true

  if az webapp show \
      --name nina-webapp-sidecar \
      --resource-group "${rg}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" &>/dev/null; then
    # Update main container image on existing webapp
    az webapp config container set \
      --name nina-webapp-sidecar \
      --resource-group "${rg}" \
      --docker-custom-image-name "${ACR_REGISTRY}/plain-app:${IMAGE_TAG}" \
      --docker-registry-server-url "https://${ACR_REGISTRY}" \
      --docker-registry-server-user "${ACR_USER}" \
      --docker-registry-server-password "${ACR_PASS}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
  else
    az webapp create \
      --name nina-webapp-sidecar \
      --resource-group "${rg}" \
      --plan dd-test-plan-sidecar \
      --deployment-container-image-name "${ACR_REGISTRY}/plain-app:${IMAGE_TAG}" \
      --https-only true \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
    az webapp config container set \
      --name nina-webapp-sidecar \
      --resource-group "${rg}" \
      --docker-registry-server-url "https://${ACR_REGISTRY}" \
      --docker-registry-server-user "${ACR_USER}" \
      --docker-registry-server-password "${ACR_PASS}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
  fi

  # All env vars in App Settings — sidecontainer inherits them automatically
  az webapp config appsettings set \
    --name nina-webapp-sidecar \
    --resource-group "${rg}" \
    --settings \
      DD_SITE="${DD_SITE}" \
      DD_SERVICE=nina-webapp-sidecar \
      DD_ENV=nina \
      DD_APM_NON_LOCAL_TRAFFIC=true \
      DD_DOGSTATSD_NON_LOCAL_TRAFFIC=true \
      DD_SERVERLESS_DIAGNOSTIC_INFO=true \
      DD_AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
      DD_AZURE_RESOURCE_GROUP="${rg}" \
      DD_LOG_LEVEL=warn \
      DD_API_KEY="${DD_API_KEY}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  _webapp_enable_sitecontainers "${rg}" nina-webapp-sidecar

  # Main app container (isMain=true, targetPort=8080) — no env vars in body
  _webapp_sitecontainer_put "${rg}" nina-webapp-sidecar main "$(python3 -c "
import json
print(json.dumps({'properties': {
  'image':          '${ACR_REGISTRY}/plain-app:${IMAGE_TAG}',
  'isMain':         True,
  'authType':       'UserCredentials',
  'userName':       '${ACR_USER}',
  'passwordSecret': '${ACR_PASS}',
  'targetPort':     '8080',
}}))
")"

  # dd-agent sidecar (isMain=false, no args → sidecar mode)
  # startUpCommand must be empty so the distroless image's ENTRYPOINT is used.
  # No environmentVariables here — inherited from App Settings above.
  _webapp_sitecontainer_put "${rg}" nina-webapp-sidecar dd-agent "$(python3 -c "
import json
print(json.dumps({'properties': {
  'image':          '${ACR_REGISTRY}/serverless-init:${IMAGE_TAG}',
  'isMain':         False,
  'authType':       'UserCredentials',
  'userName':       '${ACR_USER}',
  'passwordSecret': '${ACR_PASS}',
  'startUpCommand': '',
}}))
")"

  az webapp restart \
    --name nina-webapp-sidecar \
    --resource-group "${rg}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  WEB_APP_SIDECAR_URL=$(az webapp show \
    --name nina-webapp-sidecar \
    --resource-group "${rg}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "defaultHostName" \
    --output tsv)
  log "#8 URL: https://${WEB_APP_SIDECAR_URL}"
  export WEB_APP_SIDECAR_URL="https://${WEB_APP_SIDECAR_URL}"
}

# ---------------------------------------------------------------------------
# #9 — Azure Web App Linux Code + sidecar
# main app: Azure-managed Python 3.11 runtime (no custom Docker image)
# dd-agent sidecar: serverless-init with no args → sidecar mode
#
# Env vars go in App Settings — sidecontainer inherits them.
# WEBSITES_ENABLE_APP_SERVICE_STORAGE=true required for code apps with sidecars.
# ---------------------------------------------------------------------------
deploy_web_app_linux_code_sidecar() {
  log "=== #9 Azure Web App (Linux Code + sidecar) ==="

  _acr_creds
  local rg="${AZURE_RESOURCE_GROUP_AAS}"

  az group create \
    --name "${rg}" \
    --location "${AZURE_REGION}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null || true

  # #9 intentionally uses its OWN plan (dd-test-plan-linux-code) so that it
  # gets a different underlying VM than #8 (dd-test-plan-sidecar). Both plans
  # use the same region and SKU. Without this separation both apps report the
  # same gopsutil/DMI UUID and collapse to a single row in REDAPL.
  az appservice plan create \
    --name dd-test-plan-linux-code \
    --resource-group "${rg}" \
    --sku B2 \
    --is-linux \
    --subscription "${AZURE_SUBSCRIPTION_ID}" 2>/dev/null || true

  # If the webapp already exists on the old shared plan, move it to the new one.
  if az webapp show \
      --name nina-webapp-linux-code \
      --resource-group "${rg}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" &>/dev/null; then
    current_plan=$(az webapp show \
      --name nina-webapp-linux-code \
      --resource-group "${rg}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" \
      --query appServicePlanId -o tsv 2>/dev/null | sed 's|.*/||')
    if [[ "${current_plan}" != "dd-test-plan-linux-code" ]]; then
      log "#9 migrating nina-webapp-linux-code from plan '${current_plan}' → dd-test-plan-linux-code for distinct UUID"
      az webapp update \
        --name nina-webapp-linux-code \
        --resource-group "${rg}" \
        --plan dd-test-plan-linux-code \
        --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
    fi
  else
    az webapp create \
      --name nina-webapp-linux-code \
      --resource-group "${rg}" \
      --plan dd-test-plan-linux-code \
      --runtime "PYTHON:3.11" \
      --https-only true \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
    # Use a simple built-in HTTP server as the main app for testing
    az webapp config set \
      --name nina-webapp-linux-code \
      --resource-group "${rg}" \
      --startup-file "python3 -m http.server 8080" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null
  fi

  # All env vars in App Settings — sidecontainer inherits them automatically
  az webapp config appsettings set \
    --name nina-webapp-linux-code \
    --resource-group "${rg}" \
    --settings \
      DD_SITE="${DD_SITE}" \
      DD_SERVICE=nina-webapp-linux-code \
      DD_ENV=nina \
      DD_APM_NON_LOCAL_TRAFFIC=true \
      DD_DOGSTATSD_NON_LOCAL_TRAFFIC=true \
      DD_SERVERLESS_DIAGNOSTIC_INFO=true \
      DD_AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
      DD_AZURE_RESOURCE_GROUP="${rg}" \
      DD_LOG_LEVEL=warn \
      DD_API_KEY="${DD_API_KEY}" \
      WEBSITES_ENABLE_APP_SERVICE_STORAGE=true \
      WEBSITES_PORT=8080 \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  # dd-agent sidecar only (isMain=false) — Python runtime stays as the main app
  # No environmentVariables in body — inherited from App Settings above.
  _webapp_sitecontainer_put "${rg}" nina-webapp-linux-code dd-agent "$(python3 -c "
import json
print(json.dumps({'properties': {
  'image':          '${ACR_REGISTRY}/serverless-init:${IMAGE_TAG}',
  'isMain':         False,
  'authType':       'UserCredentials',
  'userName':       '${ACR_USER}',
  'passwordSecret': '${ACR_PASS}',
  'startUpCommand': '',
}}))
")"

  az webapp restart \
    --name nina-webapp-linux-code \
    --resource-group "${rg}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" >/dev/null

  WEB_APP_LINUX_CODE_URL=$(az webapp show \
    --name nina-webapp-linux-code \
    --resource-group "${rg}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --query "defaultHostName" \
    --output tsv)
  log "#9 URL: https://${WEB_APP_LINUX_CODE_URL}"
  export WEB_APP_LINUX_CODE_URL="https://${WEB_APP_LINUX_CODE_URL}"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  log "Starting deployment (DD_ENV=nina, IMAGE_TAG=${IMAGE_TAG})"
  log "GCP: ${APP_IMAGE}"
  log "ACR: ${AZURE_APP_IMAGE}"

  deploy_cloudrun_service              # #1 GCP Cloud Run Service (init-container)
  deploy_cloudrun_sidecar              # #2 GCP Cloud Run Service (sidecar)
  deploy_cloudrun_job                  # #3 GCP Cloud Run Job (init-container)
  deploy_cloudrun_function_sidecar     # #4 GCP Cloud Run Function gen2 (sidecar)
  deploy_container_app                 # #5 Azure Container App (init-container)
  deploy_container_app_sidecar         # #6 Azure Container App (sidecar)
  deploy_web_app_container             # #7 Azure Web App Linux Containers (init-container)
  deploy_web_app_sidecar               # #8 Azure Web App SITECONTAINERS (sidecar)
  deploy_web_app_linux_code_sidecar    # #9 Azure Web App Linux Code + sidecar

  echo ""
  echo "=========================================="
  echo "Deployment complete. Service URLs:"
  echo "  #1 GCP Cloud Run (init)            : ${CLOUDRUN_SERVICE_URL:-N/A}"
  echo "  #2 GCP Cloud Run (sidecar)         : ${CLOUDRUN_SIDECAR_URL:-N/A}"
  echo "  #3 GCP Cloud Run Job               : (no HTTP URL — job only)"
  echo "  #4 GCP Cloud Run Fn gen2 (sidecar) : ${CLOUDRUN_FUNCTION_SIDECAR_URL:-N/A}"
  echo "  #5 Azure Container App (init)      : ${CONTAINER_APP_URL:-N/A}"
  echo "  #6 Azure Container App (sidecar)   : ${CONTAINER_APP_SIDECAR_URL:-N/A}"
  echo "  #7 Azure Web App (containers)      : ${WEB_APP_CONTAINER_URL:-N/A}"
  echo "  #8 Azure Web App (sidecar)         : ${WEB_APP_SIDECAR_URL:-N/A}"
  echo "  #9 Azure Web App (Linux Code+sidecar): ${WEB_APP_LINUX_CODE_URL:-N/A}"
  echo "=========================================="
  echo ""
  echo "Next: curl each URL, then run ./check-logs.sh to inspect SERVERLESS_DIAGNOSTIC output."
}

# ---------------------------------------------------------------------------
# trigger_all — force a cold start on every service and wait for inventory
# ---------------------------------------------------------------------------
# Usage: source ./deploy.sh && trigger_all
#        Or with env vars set: ./deploy.sh trigger_all
#
# What it does:
#   1. GCP Cloud Run: deploys a no-op revision bump (forces new instance UUID)
#   2. Azure Web Apps: restarts the app (existing DMI UUID, but kicks the
#      inventory runner on the next startup — good enough to confirm payload delivery)
#   3. Azure Container Apps: restarts the replica
#   4. Waits DD_INVENTORIES_MIN_INTERVAL (default 65s) then tails check-logs.sh
#
# For REDAPL validation:
#   - Set DD_API_KEY to the dddev API key (from Vault: 'vault kv get kv/dd/api_keys/dddev')
#   - After trigger_all completes, run: check-logs.sh then query REDAPL:
#       SELECT _key AS uuid, agent_version, first_seen_at FROM datadog_agent
#       WHERE first_seen_at > '<timestamp from this run>' ORDER BY first_seen_at DESC LIMIT 20;
# ---------------------------------------------------------------------------
trigger_all() {
  : "${GCP_PROJECT:?GCP_PROJECT must be set}"
  : "${AZURE_SUBSCRIPTION_ID:?AZURE_SUBSCRIPTION_ID must be set}"
  local rg_aca="${AZURE_RG_ACA:-dd-serverless-test-aca}"
  local rg_aas="${AZURE_RG_AAS:-dd-serverless-test-aas}"
  local wait_secs="${DD_INVENTORIES_MIN_INTERVAL:-65}"

  log "=== trigger_all: forcing cold starts on all 9 services ==="
  log "Will wait ${wait_secs}s after triggers for inventory runner to fire"
  log "Start time: $(date -u '+%Y-%m-%dT%H:%M:%SZ') — use this in your REDAPL WHERE clause"

  # --- GCP Cloud Run: update env var to force a new revision (new container = new UUID) ---
  local stamp="trigger-$(date -u +%Y%m%d%H%M%S)"
  for svc in nina-cloudrun-init nina-cloudrun-sidecar nina-cloudrun-function-sidecar; do
    log "GCP: redeploying ${svc} (new revision → new UUID)"
    gcloud run services update "${svc}" \
      --project="${GCP_PROJECT}" \
      --region=us-central1 \
      --update-env-vars "TRIGGER_STAMP=${stamp}" \
      --quiet 2>/dev/null || log "  (warning: ${svc} update failed — may not exist)"
    # Trigger an HTTP request to force the new revision to start
    local url
    url=$(gcloud run services describe "${svc}" --project="${GCP_PROJECT}" \
      --region=us-central1 --format='value(status.url)' 2>/dev/null || true)
    [[ -n "${url}" ]] && curl -sf --max-time 10 "${url}" >/dev/null 2>&1 \
      && log "  triggered: ${url}" \
      || log "  (no URL or request failed — instance may still start)"
  done

  # Cloud Run Job: execute one run
  log "GCP: executing nina-cloudrun-job (new execution = new UUID)"
  gcloud run jobs execute nina-cloudrun-job \
    --project="${GCP_PROJECT}" \
    --region=us-central1 \
    --quiet 2>/dev/null || log "  (warning: job execute failed — may not exist)"

  # --- Azure Container Apps: restart replicas ---
  for app in nina-containerapp-init nina-containerapp-sidecar; do
    log "Azure CA: restarting ${app}"
    az containerapp revision restart \
      --name "${app}" \
      --resource-group "${rg_aca}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" \
      --revision "$(az containerapp show --name "${app}" --resource-group "${rg_aca}" \
        --subscription "${AZURE_SUBSCRIPTION_ID}" \
        --query 'properties.latestRevisionName' -o tsv 2>/dev/null)" \
      2>/dev/null || log "  (warning: restart failed — trying webapp restart instead)"
  done

  # --- Azure Web Apps: restart (existing DMI UUID, but starts fresh Fx app) ---
  for app in nina-webapp-container nina-webapp-sidecar nina-webapp-linux-code; do
    log "Azure AAS: restarting ${app}"
    az webapp restart \
      --name "${app}" \
      --resource-group "${rg_aas}" \
      --subscription "${AZURE_SUBSCRIPTION_ID}" \
      2>/dev/null || log "  (warning: restart failed for ${app})"
  done

  log "All triggers sent. Waiting ${wait_secs}s for inventory runner to fire..."
  sleep "${wait_secs}"

  log "=== Collecting logs ==="
  local script_dir
  script_dir="$(dirname "${BASH_SOURCE[0]:-$0}")"
  GCP_PROJECT="${GCP_PROJECT}" \
  AZURE_SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}" \
    "${script_dir}/check-logs.sh"
}

# Allow sourcing the file to call individual functions without running main.
# Usage: source ./deploy.sh && deploy_web_app_sidecar
#        source ./deploy.sh && trigger_all
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
  if [[ "${1:-}" == "trigger_all" ]]; then
    trigger_all
  else
    main "$@"
  fi
fi
