# Usage: read_ci_secret.ps1 <secretName> <secretField>
#
# Reads a DataDog/datadog-agent CI secret from Vault with the job's CI Identity.
#
# `ci-identities-gitlab-job-client secrets read` builds the secret path from the
# caller's repository, so it fails from forks such as ddoghq/datadog-agent, which
# read the secrets of DataDog/datadog-agent. Use the full path instead.
# See https://datadoghq.atlassian.net/browse/SINT-5932
param (
    [Parameter(Mandatory = $true)][string]$secretName,
    [Parameter(Mandatory = $true)][string]$secretField
)

$secretsRepository = "datadog/datadog-agent"

$ciIdentitiesExe = "C:\devtools\ci-identities-gitlab-job-client.exe"
if (-not (Test-Path $ciIdentitiesExe)) {
    $ciIdentitiesExe = "C:\ci-identities-gitlab-job-client.exe"
}

# Talk to Vault directly, like `secrets read` does. A preset VAULT_ADDR can point to a
# proxy, such as the Emissary sidecar, which replaces our CI Identity token with its own.
$env:VAULT_ADDR = "https://vault.us1.ddbuild.io"

# CI Identities grants trusted-jobs to protected refs. The pinned client predates
# `token-claims`, so guess from GitLab and fall back to the other trust level:
# a CI Identity can only read the secrets of its own trust level.
if ($env:CI_COMMIT_REF_PROTECTED -eq "true") {
    $trustLevels = @("trusted-jobs", "untrusted-jobs")
} else {
    $trustLevels = @("untrusted-jobs", "trusted-jobs")
}

foreach ($trustLevel in $trustLevels) {
    $result = & $ciIdentitiesExe vault env -- vault kv get "-field=$secretField" "kv/ci/$secretsRepository/$trustLevel/$secretName"
    if ($LASTEXITCODE -eq 0) {
        $result
        exit 0
    }
}
exit 1
