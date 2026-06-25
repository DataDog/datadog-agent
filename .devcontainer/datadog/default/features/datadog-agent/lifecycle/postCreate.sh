#!/bin/bash


# Install Claude MCPs
# Datadog
claude mcp add --transport http datadog-mcp https://mcp.datadoghq.com/api/unstable/mcp-server/mcp?toolsets=all --scope user

# Atlassian
claude mcp add --transport http --scope user atlassian https://mcp.atlassian.com/v1/mcp

# Google
claude mcp add datadog-google-workspace --transport http https://google-workspace-mcp-server-834963730936.us-central1.run.app/mcp --scope user

# DDCI
claude mcp add --transport http "ddci-mcp-prod" 'https://ddci-mcp.mcp.us1.ddbuild.io/internal/mcp' --scope user

# Start the developer environment
DEV_ENV_IMAGE=$(cat /opt/doghome/devcontainer/dev-env-image)
dda env dev start --image "${DEV_ENV_IMAGE}" --no-pull 2>&1 | tee "/home/bits/.dev-env-start.log"
