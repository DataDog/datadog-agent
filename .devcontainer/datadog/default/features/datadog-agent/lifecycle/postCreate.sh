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

# Run install tools
cd ~/dd/datadog-agent

# Tweaking environment variables, will be removed once the Docker image is updated.
# Make sure we can dynamically install dda dependencies
export DDA_NO_DYNAMIC_DEPS=0
# Unset GOPATH otherwise it is set to /go in the build image base
unset GOPATH

dda inv install-tools 2>&1 | tee "/home/bits/.install-tools.log"
dda inv vscode.setup 2>&1 | tee "/home/bits/.vscode-setup.log"
