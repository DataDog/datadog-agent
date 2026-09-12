#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_AGENT_SRC, AIX_GIT_REMOTE, CI_COMMIT_SHA, and CI_COMMIT_REF_NAME
# being set. Reuses any existing clone to avoid cloning from scratch each run.

echo "=== AIX: cloning agent source at $CI_COMMIT_SHA ==="
aix_run_cmd <<EOF
set -eu
export PATH=/opt/freeware/bin:/usr/bin:\$PATH
if [ ! -d "$AIX_AGENT_SRC/.git" ]; then
    git clone --progress "$AIX_GIT_REMOTE" "$AIX_AGENT_SRC"
fi
cd "$AIX_AGENT_SRC"
git clean -df
git reset --hard
git fetch --progress origin
# The commit under test may not be reachable from origin yet (e.g. a
# brand-new branch push); fetch the branch ref as a fallback.
git fetch --progress origin "$CI_COMMIT_REF_NAME" || true
git checkout "$CI_COMMIT_SHA"
EOF
