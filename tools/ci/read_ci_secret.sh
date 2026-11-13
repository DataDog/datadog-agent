#!/bin/bash

# Usage: read_ci_secret.sh <secret-name> <secret-field>
#
# Reads a DataDog/datadog-agent CI secret from Vault with the job's CI Identity.
#
# `ci-identities-gitlab-job-client secrets read` builds the secret path from the
# caller's repository, so it fails from forks such as ddoghq/datadog-agent, which
# read the secrets of DataDog/datadog-agent. Use the full path instead.
# See https://datadoghq.atlassian.net/browse/SINT-5932

set -euo pipefail
set +x

secret_name="$1"
secret_field="$2"
secrets_repository="datadog/datadog-agent"

# Talk to Vault directly, like `secrets read` does. On Kubernetes runners VAULT_ADDR
# points to the Emissary sidecar, which replaces our CI Identity token with the pod's.
export VAULT_ADDR="https://vault.us1.ddbuild.io"

# CI Identities grants trusted-jobs to protected refs. The pinned client predates
# `token-claims`, so guess from GitLab and fall back to the other trust level:
# a CI Identity can only read the secrets of its own trust level.
if [ "${CI_COMMIT_REF_PROTECTED:-false}" = "true" ]; then
    trust_levels="trusted-jobs untrusted-jobs"
else
    trust_levels="untrusted-jobs trusted-jobs"
fi

# shellcheck disable=SC2016 # expanded by the inner shell
ci-identities-gitlab-job-client vault env -- bash -c '
    for trust_level in $1; do
        vault kv get -field="$2" "kv/ci/$3/${trust_level}/$4" && exit 0
    done
    exit 1
' _ "${trust_levels}" "${secret_field}" "${secrets_repository}" "${secret_name}"
