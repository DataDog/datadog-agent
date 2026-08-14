# SVLS-9526 — Serverless Init Inventory POC

End-to-end proof of concept for [SVLS-9526]: serverless-init agents appear in
the `datadog_agent` REDAPL table when `ForceCollect()` fires on cold start.

Related PRs:
- **#54537** — `ForceCollect()`: send inventory payload on every cold start
- **#54538** — `inventories_first_run_delay` config key
- **#54543** — `cmd/serverless-init/inventory`: `serverless_*` fields in payload

## Services under test (9 total)

| # | Platform | Name | Mode |
|---|---|---|---|
| 1 | GCP Cloud Run Service | `nina-cloudrun-init` | init-container |
| 2 | GCP Cloud Run Service | `nina-cloudrun-sidecar` | sidecar |
| 3 | GCP Cloud Run Job | `nina-cloudrun-job` | init-container |
| 4 | GCP Cloud Run Function gen2 | `nina-cloudrun-function-sidecar` | sidecar |
| 5 | Azure Container App | `nina-containerapp-init` | init-container |
| 6 | Azure Container App | `nina-containerapp-sidecar` | sidecar |
| 7 | Azure Web App (Linux Containers) | `nina-webapp-container` | init-container |
| 8 | Azure Web App (SITECONTAINERS) | `nina-webapp-sidecar` | sidecar |
| 9 | Azure Web App (Linux Code) | `nina-webapp-linux-code` | sidecar |

## Prerequisites

Install:
- `gcloud` (Google Cloud SDK) — `gcloud auth login` before running
- `az` (Azure CLI) — `az login` before running
- `docker` with buildx for `--platform linux/amd64`
- `python3`, `envsubst` (from `gettext`)

## Quick start — one command

```bash
export GCP_PROJECT=datadog-serverless-gcp-demo
export AZURE_SUBSCRIPTION_ID=<your-subscription-id>
export DD_API_KEY=$(vault kv get -field=api_key kv/dd/api_keys/dddev)
export IMAGE_TAG=1.10.2-poc-$(date +%Y%m%d)   # or any tag

cd scripts/serverless-deploy
./demo.sh
```

`demo.sh` runs all three steps in sequence:
1. **Build** — compiles serverless-init from the current branch and pushes to GCR + ACR
2. **Deploy** — creates/updates all 9 services across GCP and Azure
3. **Trigger** — forces a cold start on each service, waits 75s, then collects
   `SERVERLESS_DIAGNOSTIC` logs and prints the REDAPL SQL query

## Step-by-step (if you need to run stages individually)

### Step 1 — Build and push images

```bash
GCP_PROJECT=datadog-serverless-gcp-demo \
AZURE_SUBSCRIPTION_ID=<sub-id> \
IMAGE_TAG=1.10.2-poc \
./build-image.sh
```

Pushes three images to GCR Artifact Registry (and ACR if `AZURE_SUBSCRIPTION_ID` is set):
- `serverless-init:<tag>` — built from current branch source
- `test-app:<tag>` — Flask app with serverless-init as entrypoint (init-container mode)
- `plain-app:<tag>` — Flask app only, no serverless-init (sidecar mode)

### Step 2 — Deploy all 9 services

```bash
GCP_PROJECT=datadog-serverless-gcp-demo \
DD_API_KEY=<key> \
AZURE_SUBSCRIPTION_ID=<sub-id> \
IMAGE_TAG=1.10.2-poc \
./deploy.sh
```

### Step 3 — Trigger cold starts + collect logs

```bash
GCP_PROJECT=datadog-serverless-gcp-demo \
AZURE_SUBSCRIPTION_ID=<sub-id> \
./deploy.sh trigger_all
```

Triggers a cold start on each service, waits 75s for the inventory runner to
fire, then calls `check-logs.sh` which:
- Pulls `SERVERLESS_DIAGNOSTIC` lines from GCP Cloud Logging and Azure log streams
- Extracts `uuid (agent):` values from each service
- Prints a REDAPL SQL query to validate the payloads in `datadog_agent`

### Step 4 — Validate in REDAPL

Paste the generated query into **go/redapl → Queries → SQL**.

Expected results:

| Platform | UUID behaviour | Rows in `datadog_agent` |
|---|---|---|
| GCP Cloud Run | New UUID per cold start | 1 new row per trigger |
| Azure Container App | Stable UUID per replica | Upserts same row |
| Azure Web App | DMI UUID from underlying VM | 1 row per App Service Plan VM |

The Azure App Service finding (two apps on the same Plan share a UUID) is the
primary motivation for `serverless_init_agent` — a separate table keyed by
CCRID (`resource_id`) instead of Agent UUID.

## Skip flags

```bash
SKIP_BUILD=true ./demo.sh    # use an already-pushed image
SKIP_DEPLOY=true ./demo.sh   # only trigger + collect logs (services already deployed)
```

## Directory structure

```
scripts/serverless-deploy/
  demo.sh                      # One-command end-to-end runner (build → deploy → trigger)
  deploy.sh                    # Deploy all 9 services; trigger_all subcommand
  build-image.sh               # Build + push serverless-init, test-app, plain-app images
  check-logs.sh                # Collect SERVERLESS_DIAGNOSTIC logs + generate REDAPL SQL
  Dockerfile.serverless-init   # Builds serverless-init from branch source
  app/
    app.py                     # Flask hello-world (DogStatsD + structured logs)
    requirements.txt
    Dockerfile                 # test-app: serverless-init wraps Flask app
    Dockerfile.plain           # plain-app: Flask app only (for sidecar deployments)
    cloudrun-sidecar.yaml      # Knative multi-container spec (#2)
    cloudrun-function-sidecar.yaml  # Knative multi-container spec (#4)
```
