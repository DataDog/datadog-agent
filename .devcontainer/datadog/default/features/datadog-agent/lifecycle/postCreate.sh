#!/bin/bash
set -euo pipefail

configure_claude_mcp() {
    if ! command -v claude >/dev/null 2>&1; then
        return 0
    fi

    claude mcp add --transport http datadog-mcp https://mcp.datadoghq.com/api/unstable/mcp-server/mcp?toolsets=all --scope user || true
    claude mcp add --transport http --scope user atlassian https://mcp.atlassian.com/v1/mcp || true
    claude mcp add datadog-google-workspace --transport http https://google-workspace-mcp-server-834963730936.us-central1.run.app/mcp --scope user || true
    claude mcp add --transport http "ddci-mcp-prod" 'https://ddci-mcp.mcp.us1.ddbuild.io/internal/mcp' --scope user || true
}

repo_dir="${HOME}/go/src/github.com/DataDog/datadog-agent"

ln -s "${HOME}/go/src/github.com/DataDog" "${HOME}/dd"

if [[ ! -d "${repo_dir}" ]]; then
    echo "Unable to find datadog-agent checkout at ${repo_dir}" >&2
    exit 1
fi

configure_claude_mcp

cd "${repo_dir}"

DDA_NO_DYNAMIC_DEPS=0 dda inv install-tools 2>&1 | tee "${HOME}/.install-tools.log"
