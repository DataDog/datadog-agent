#!/bin/bash
# Prints a short-lived GitLab API token for DataDog/datadog-agent to stdout.
#
# Mirrors main's `get_gitlab_token` (tasks/libs/ciproviders/gitlab_api.py): mints
# an infra bearer token with `authanywhere` and exchanges it for a scoped GitLab
# token at the BTI CI API. Replaces the expired static scheduler token previously
# read from SSM.
#
# Requires `authanywhere` on PATH (see install_authanywhere.sh).
set -euo pipefail

owner="${GITLAB_TOKEN_OWNER:-DataDog}"
repository="${GITLAB_TOKEN_REPOSITORY:-datadog-agent}"
audience="${GITLAB_TOKEN_AUDIENCE:-sdm}"

# authanywhere prints an "Authorization: Bearer <token>" header line.
infra_header="$(authanywhere --audience "$audience")"
case "$infra_header" in
    "Authorization: Bearer "*) ;;
    "Bearer "*) infra_header="Authorization: $infra_header" ;;
    *) echo "authanywhere returned an unexpected token format" >&2; exit 1 ;;
esac

url="https://bti-ci-api.us1.ddbuild.io/internal/ci/gitlab/token?owner=${owner}&repository=${repository}"

response="$(curl -fsSL --retry 2 -H "$infra_header" "$url")"

token="$(printf '%s' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
if [ -z "$token" ]; then
    echo "Failed to extract GitLab token from BTI CI API response" >&2
    exit 1
fi

printf '%s' "$token"
