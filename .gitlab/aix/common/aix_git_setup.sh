#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_AGENT_SRC, AIX_GIT_REMOTE, CI_COMMIT_SHA, and CI_COMMIT_REF_NAME
# being set. Reuses any existing clone to avoid cloning from scratch each run.

# GitHub may temporarily require authentication for *unauthenticated* git over
# HTTPS on public repos (GitHub changelog, 2026-10-19: "Some Git over HTTPS
# may temporarily require authentication"). When that happens, the host's
# anonymous fetches get a 401 and, with a PTY on stdin (aix_run_cmd uses
# ssh -tt), git hangs waiting for a username until the 2h job timeout.
#
# The CI runner mints a short-lived (1h) read-only GitHub token (dd-octo-sts,
# see .aix_remote's before_script) and aix_run_cmd forwards it to the host as
# GITHUB_TOKEN — it travels in the job environment and in the aix_run_cmd
# script files (mode 600, private to the CI user; see aix_helpers.sh), never
# in git's config on disk. Install a credential helper that feeds it to git:
# with an empty GITHUB_TOKEN the helper yields no credentials and the fetch
# stays anonymous (failing fast, thanks to GIT_TERMINAL_PROMPT=0 exported by
# aix_run_cmd). Any authenticated token can read public repos, so the single
# token covers the agent, integrations-core and saluki fetches.

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
git clean -df
git reset --hard
git fetch --progress origin
# The commit under test may not be reachable from origin yet (e.g. a
# brand-new branch push); fetch the branch ref as a fallback.
git fetch --progress origin "$CI_COMMIT_REF_NAME" || true
git checkout "$CI_COMMIT_SHA"
EOF
