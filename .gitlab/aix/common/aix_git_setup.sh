#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_AGENT_SRC, AIX_GIT_REMOTE, AIX_GIT_TOKEN_FILE, CI_COMMIT_SHA,
# CI_COMMIT_REF_NAME, and the aix_* helpers being set.
#
# Each GitHub repository gets its own clone (see AIX_AGENT_SRC in aix_select_repo),
# so commits under embargo in the ddoghq/datadog-agent mirror never land in the
# clone used by public DataDog/datadog-agent pipelines. A clone whose origin does
# not match the expected remote is wiped and recloned rather than repointed, so it
# can't keep objects fetched from the other repository.
#
# The private mirror is fetched with a short-lived GitHub token minted on the
# runner. It is written to AIX_GIT_TOKEN_FILE on the host and removed as soon as
# the fetch ends (and again in after_script). It is never stored in the git config.

if [ "$AIX_REPO_PRIVATE" = "true" ]; then
    echo "=== AIX: minting a read-only GitHub token for $AIX_GITHUB_REPO ==="
    (
        set +x
        token=$(dd-octo-sts token --scope "$AIX_GITHUB_REPO" --policy self.gitlab.aix-clone) || exit 1
        printf '%s' "$token" | aix_write_secret "$AIX_GIT_TOKEN_FILE"
    ) || { echo "Failed to mint or ship the GitHub token" >&2; exit 1; }
fi

echo "=== AIX: cloning $AIX_GITHUB_REPO at $CI_COMMIT_SHA in $AIX_AGENT_SRC ==="
aix_run_cmd <<EOF
set -eu
export PATH=/opt/freeware/bin:/usr/bin:\$PATH

if [ -f "$AIX_GIT_TOKEN_FILE" ]; then
    trap 'rm -f "$AIX_GIT_TOKEN_FILE"' EXIT
    # Pass the credential through the environment (git >= 2.31) so it is never
    # written to .git/config.
    export GIT_CONFIG_COUNT=1
    export GIT_CONFIG_KEY_0="http.https://github.com/.extraheader"
    export GIT_CONFIG_VALUE_0="AUTHORIZATION: basic \$(printf 'x-access-token:%s' "\$(cat "$AIX_GIT_TOKEN_FILE")" | openssl base64 -A)"
fi

if [ -d "$AIX_AGENT_SRC/.git" ] && [ "\$(git -C "$AIX_AGENT_SRC" remote get-url origin 2>/dev/null)" != "$AIX_GIT_REMOTE" ]; then
    echo "Existing clone does not track $AIX_GIT_REMOTE, removing it"
    rm -rf "$AIX_AGENT_SRC"
fi
if [ ! -d "$AIX_AGENT_SRC/.git" ]; then
    mkdir -p "\$(dirname "$AIX_AGENT_SRC")"
    git clone --progress "$AIX_GIT_REMOTE" "$AIX_AGENT_SRC"
fi
cd "$AIX_AGENT_SRC"
git clean -df
git reset --hard
git fetch --progress origin
# The commit under test may not be reachable from origin yet (e.g. a
# brand-new branch push); fetch the branch ref as a fallback.
git fetch --progress origin "$CI_COMMIT_REF_NAME" || true
git checkout --detach "$CI_COMMIT_SHA"
EOF
