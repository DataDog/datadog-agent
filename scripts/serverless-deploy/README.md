# SVLS-9526 Serverless Diagnostic Deployment

Deploys 5 test services across GCP and Azure to capture what env vars and metadata
are visible to `serverless-init` on each platform, using a custom image built from
this branch (with `DD_SERVERLESS_DIAGNOSTIC_INFO=true` logging enabled).

## Services deployed

| Platform | Name | Notes |
|---|---|---|
| GCP Cloud Run Service | `dd-test-cloudrun-service` | HTTP, auto-scales |
| GCP Cloud Run Job | `dd-test-cloudrun-job` | Runs once, no HTTP |
| GCP Cloud Run Function v2 | `dd-test-cloudrun-function` | HTTP trigger |
| Azure Container App | `dd-test-container-app` | HTTP, external ingress |
| Azure Web App (Linux) | `dd-test-web-app` | HTTP, Python 3.11 |

## Prerequisites

Install the following CLIs:
- `gcloud` (Google Cloud SDK)
- `az` (Azure CLI)
- `docker` (with buildx for `--platform linux/amd64`)

Set the following environment variables:

```bash
export GCP_PROJECT=<your-gcp-project-id>
export AZURE_SUBSCRIPTION_ID=<your-azure-subscription-id>

# Fetch the API key from vault — do NOT hardcode it
export DD_API_KEY=$(vault kv get -field=api_key secret/dd/serverless-test)
```

Optional overrides (defaults shown):

```bash
export DD_SITE=datadoghq.com
export GCP_REGION=us-central1
export AZURE_REGION=eastus
export IMAGE_TAG=latest
```

## Step 1 — Build and push images

Builds the custom `serverless-init` binary from the current branch source and the
test Flask app, then pushes both to GCR Artifact Registry.

```bash
./build-image.sh
```

This creates (if needed) an Artifact Registry repo named `serverless-test` in your
GCP project and pushes:
- `<registry>/serverless-init:<IMAGE_TAG>`
- `<registry>/test-app:<IMAGE_TAG>`

## Step 2 — Deploy all 5 services

```bash
./deploy.sh
```

Deploys in sequence: Cloud Run Service, Cloud Run Job, Cloud Run Function v2,
Azure Container App, Azure Web App. Prints URLs on completion.

## Step 3 — Trigger each service

Send a request to each HTTP service to generate diagnostic output:

```bash
curl "${CLOUDRUN_SERVICE_URL}"
curl "${CLOUDRUN_FUNCTION_URL}"
curl "${CONTAINER_APP_URL}"
curl "${WEB_APP_URL}"
# Cloud Run Job runs automatically during deploy (no HTTP endpoint)
```

## Step 4 — Check diagnostic logs

```bash
./check-logs.sh
```

Queries GCP Cloud Logging and Azure log streams for lines containing
`SERVERLESS_DIAGNOSTIC` — the structured output emitted by `serverless-init`
when `DD_SERVERLESS_DIAGNOSTIC_INFO=true` is set.

## Directory structure

```
scripts/serverless-deploy/
  deploy.sh                    # Main deploy script (all 5 platforms)
  build-image.sh               # Build + push serverless-init and test-app images
  check-logs.sh                # Fetch SERVERLESS_DIAGNOSTIC output from all platforms
  Dockerfile.serverless-init   # Builds serverless-init from branch source
  app/
    app.py                     # Flask hello-world app (DogStatsD + structured logs)
    requirements.txt
    Dockerfile                 # Test app container image
    function/
      main.py                  # Cloud Run Function v2 handler
      requirements.txt
```
