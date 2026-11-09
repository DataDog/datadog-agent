#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_USER, AIX_HOST, CI_PROJECT_PATH, CI_PIPELINE_ID, and CI_JOB_ID being set.

export AIX_SSH_COMMON_OPTS="-o ServerAliveInterval=20 -o ServerAliveCountMax=6"
export AIX_SSH_DEST="$AIX_USER@$AIX_HOST"

# Non-sensitive CI/GitLab environment variables forwarded to the AIX host, so
# that remote commands run with the pipeline context. Don't forward the whole environment
# to avoid passing secrets, which are only needed in the CI runner.
AIX_FORWARDED_ENV_VARS=(
    CI_PIPELINE_ID
    CI_PIPELINE_SOURCE
    CI_PROJECT_NAME
    CI_COMMIT_SHA
    CI_COMMIT_REF_NAME
    CI_COMMIT_TAG
    BUCKET_BRANCH
    DEPLOY_AGENT
    DEPLOY_INSTALLER
)

# Prints export lines for the forwarded variables that are set in the
# current environment, safely single-quoted for the remote shell.
aix_ci_env_exports() {
    local var value
    for var in "${AIX_FORWARDED_ENV_VARS[@]}"; do
        value="${!var:-}"
        if [ -n "$value" ]; then
            printf "export %s='%s'\n" "$var" "${value//\'/\'\\\'\'}"
        fi
    done
}

# Run a script on the AIX host. Accepts the script either as inline arguments
# or on stdin, eg.:
#   aix_run_cmd echo "Hello from AIX"
#   aix_run_cmd <<EOF
#     ...
#   EOF
aix_run_cmd() {
    local script
    if [ "$#" -gt 0 ]; then
        script="$*"
    else
        script=$(cat)
    fi
    # Prepend the forwarded CI environment so the remote script runs with the
    # pipeline context.
    script="$(aix_ci_env_exports)
$script"
    local encoded
    encoded=$(printf '%s' "$script" | base64 -w0)
    # pass the script encoded in base64 to avoid quoting and stdin issues:
    # - base64 makes the payload safe to embed in the ssh command line
    # - the script is decoded to a temp file and run with `bash <file>` so
    #   that stdin remains the PTY
    # the script file is kept on failure to help with debugging
    local script_file
    script_file="/tmp/aix_run_cmd_${CI_PIPELINE_ID}_${CI_JOB_ID}_$(date +%s).sh"
    ssh -tt $AIX_SSH_COMMON_OPTS $AIX_SSH_DEST \
        "echo $encoded | openssl enc -base64 -d -A > '$script_file' && bash '$script_file'; rc=\$?; [ \$rc -eq 0 ] && rm -f '$script_file' || echo \"script kept at $script_file for debugging\"; exit \$rc"
}

copy_file_from_aix() { scp $AIX_SSH_COMMON_OPTS "$AIX_SSH_DEST:$1" "$2"; }
copy_file_to_aix() { scp $AIX_SSH_COMMON_OPTS "$1" "$AIX_SSH_DEST:$2"; }

# Selects the GitHub repository to build from the pipeline's GitLab project.
# DataDog/datadog-agent is public. ddoghq/datadog-agent is its private mirror,
# used to prepare fixes under embargo: its source must stay out of the public
# clone, so it gets its own clone directory and authenticated fetches.
aix_select_repo() {
    case "$CI_PROJECT_PATH" in
        DataDog/datadog-agent)
            AIX_GITHUB_REPO="DataDog/datadog-agent"
            AIX_REPO_PRIVATE=false
            AIX_AGENT_SRC="/opt/dd-build/datadog-agent"
            ;;
        ddoghq/datadog-agent)
            AIX_GITHUB_REPO="ddoghq/datadog-agent"
            AIX_REPO_PRIVATE=true
            AIX_AGENT_SRC="/opt/dd-build/ddoghq/datadog-agent"
            ;;
        *)
            echo "AIX jobs don't support GitLab project $CI_PROJECT_PATH" >&2
            return 1
            ;;
    esac
    AIX_GIT_REMOTE="https://github.com/$AIX_GITHUB_REPO.git"
    AIX_GIT_TOKEN_FILE="/opt/dd-build/.ci-secrets/github_token_${CI_JOB_ID}"
    export AIX_GITHUB_REPO AIX_REPO_PRIVATE AIX_AGENT_SRC AIX_GIT_REMOTE AIX_GIT_TOKEN_FILE
}

# Writes stdin to a file on the AIX host, readable by its owner only. Unlike
# aix_run_cmd, the content is never part of a script file kept for debugging.
aix_write_secret() {
    ssh $AIX_SSH_COMMON_OPTS $AIX_SSH_DEST \
        "umask 077 && mkdir -p '$(dirname "$1")' && cat > '$1'"
}

# Host-wide lock. The resource_group only serializes jobs within one GitLab
# project, but jobs from DataDog/datadog-agent and ddoghq/datadog-agent share the
# host and /opt/dd-build. mkdir is atomic; a lock older than the job timeout is
# considered stale and taken over.
AIX_LOCK_DIR=/opt/dd-build/.ci-lock
AIX_LOCK_STALE_SECONDS=$((150 * 60))
AIX_LOCK_WAIT_SECONDS=$((90 * 60))

aix_acquire_host_lock() {
    echo "=== AIX: acquiring the host lock ==="
    aix_run_cmd <<EOF
set -eu
start=\$(date +%s)
while ! mkdir "$AIX_LOCK_DIR" 2>/dev/null; do
    now=\$(date +%s)
    owner=\$(cat "$AIX_LOCK_DIR/owner" 2>/dev/null || echo "unknown 0 0")
    since=\$(echo "\$owner" | awk '{print \$3}')
    if [ \$((now - since)) -gt $AIX_LOCK_STALE_SECONDS ]; then
        echo "Taking over stale lock: \$owner"
        rm -rf "$AIX_LOCK_DIR"
        continue
    fi
    if [ \$((now - start)) -gt $AIX_LOCK_WAIT_SECONDS ]; then
        echo "Timed out waiting for the host lock held by: \$owner" >&2
        exit 1
    fi
    echo "Host busy (\$owner), waiting..."
    sleep 30
done
echo "$CI_PROJECT_PATH $CI_JOB_ID \$(date +%s)" > "$AIX_LOCK_DIR/owner"
EOF
}

# Releases the lock only if this job holds it.
aix_release_host_lock() {
    aix_run_cmd <<EOF
if [ "\$(awk '{print \$2}' "$AIX_LOCK_DIR/owner" 2>/dev/null)" = "$CI_JOB_ID" ]; then
    rm -rf "$AIX_LOCK_DIR"
fi
EOF
}
