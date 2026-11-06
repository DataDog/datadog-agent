#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_AGENT_SRC, AIX_GIT_REMOTE, CI_COMMIT_SHA, and CI_COMMIT_REF_NAME
# being set. Reuses any existing clone to avoid cloning from scratch each run.

# GitHub may require auth for HTTPS on public repos, provide a token to avoid being blocked
# https://github.blog/changelog/2026-10-19-some-git-over-https-may-temporarily-require-authentication/

echo "=== AIX: installing GitHub credential helper on the host ==="
aix_run_cmd <<'EOF'
set -eu
export PATH=/opt/freeware/bin:/usr/bin:$PATH
git config --global credential.https://github.com.helper '!f() { test -n "$GITHUB_TOKEN" || exit 0; printf "username=x-access-token\npassword=%s\n" "$GITHUB_TOKEN"; }; f'
EOF

echo "=== AIX: cloning agent source at $CI_COMMIT_SHA ==="
aix_run_cmd <<EOF
set -eu
export PATH=/opt/freeware/bin:/usr/bin:\$PATH
if [ ! -d "$AIX_AGENT_SRC/.git" ]; then
    git clone --progress "$AIX_GIT_REMOTE" "$AIX_AGENT_SRC"
fi
cd "$AIX_AGENT_SRC"
# The clone is persistent and the host is manually managed, so its origin
# can have been changed on the host; always re-point it at the remote under
# test before fetching.
git remote set-url origin "$AIX_GIT_REMOTE"
git clean -df
git reset --hard
git fetch --progress origin
# The commit under test may not be reachable from origin yet (e.g. a
# brand-new branch push); fetch the branch ref as a fallback.
git fetch --progress origin "$CI_COMMIT_REF_NAME" || true
git checkout "$CI_COMMIT_SHA"
EOF
