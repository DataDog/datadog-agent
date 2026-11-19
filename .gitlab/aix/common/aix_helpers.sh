#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_USER, AIX_HOST, CI_PIPELINE_ID, and CI_JOB_ID being set.

export AIX_SSH_COMMON_OPTS="-o ServerAliveInterval=20 -o ServerAliveCountMax=6"
export AIX_SSH_DEST="$AIX_USER@$AIX_HOST"

# Non-sensitive CI/GitLab environment variables forwarded to the AIX host, so
# that remote commands run with the pipeline context. Don't forward the whole environment
# to avoid passing secrets, which are only needed in the CI runner.
# GITHUB_TOKEN is a short-lived read-only token to fetch from public repos
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
    GITHUB_TOKEN
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

# Refresh the GitHub token when it may be stale
AIX_GITHUB_TOKEN_MAX_AGE=1800

aix_refresh_github_token() {
    local now new_token
    now=$(date +%s)
    if [ -z "${AIX_GITHUB_TOKEN_MINTED_AT:-}" ] \
        || [ $((now - AIX_GITHUB_TOKEN_MINTED_AT)) -ge "$AIX_GITHUB_TOKEN_MAX_AGE" ]; then
        if new_token=$(dd-octo-sts token --scope "${CI_PROJECT_PATH}" --policy self.gitlab.read 2>/dev/null) \
            && [ -n "$new_token" ]; then
            export GITHUB_TOKEN="$new_token"
            export AIX_GITHUB_TOKEN_MINTED_AT="$now"
        fi
    fi
}

# Run a script on the AIX host. Accepts the script either as inline arguments
# or on stdin, eg.:
#   aix_run_cmd echo "Hello from AIX"
#   aix_run_cmd <<EOF
#     ...
#   EOF
# A lost connection (ssh exit 255) triggers up to 3 attempts in total; any
# other exit code is the remote command's own status and is not retried.
aix_run_cmd() {
    local script rc attempt
    if [ "$#" -gt 0 ]; then
        script="$*"
    else
        script=$(cat)
    fi
    for attempt in 1 2 3; do
        # Re-mint the GitHub token on every attempt: it is embedded in the
        # script sent to the host, and a long command can outlive the token.
        aix_refresh_github_token
        rc=0
        _aix_run_cmd_once "$script" || rc=$?
        if [ "$rc" -ne 255 ]; then
            return "$rc"
        fi
        echo "=== AIX: connection lost (attempt $attempt/3), retrying ===" >&2
        _aix_kill_stale_scripts
        if [ "$attempt" -lt 3 ]; then sleep $((attempt * 15)); fi
    done
    return "$rc"
}

# Single aix_run_cmd attempt: assemble and send the script.
_aix_run_cmd_once() {
    local script encoded script_file
    script="$1"
    # Prepend the forwarded CI environment so the remote script runs with the
    # pipeline context.
    script="$(aix_ci_env_exports)
$script"
    # Disable interactive prompts
    script="export GIT_TERMINAL_PROMPT=0
$script"
    encoded=$(printf '%s' "$script" | base64 -w0)
    # pass the script encoded in base64 to avoid quoting and stdin issues:
    # - base64 makes the payload safe to embed in the ssh command line
    # - the script is decoded to a temp file and run with `bash <file>` so
    #   that stdin remains the PTY
    # the script file is kept on failure to help with debugging
    script_file="/tmp/aix_run_cmd_${CI_PIPELINE_ID}_${CI_JOB_ID}_$(date +%s).sh"
    # shellcheck disable=SC2086  # AIX_SSH_COMMON_OPTS intentionally word-splits into multiple ssh options
    ssh -tt $AIX_SSH_COMMON_OPTS $AIX_SSH_DEST \
        "echo $encoded | openssl enc -base64 -d -A > '$script_file' && bash '$script_file'; rc=\$?; [ \$rc -eq 0 ] && rm -f '$script_file' || echo \"script kept at $script_file for debugging\"; exit \$rc"
}

# After a connection drop, the host can keep a dead session's script running
# until its own TCP retransmit timeout fires. Kill any surviving script and
# its process group (a whole ssh -tt session shares one group) so a retry
# never runs concurrently with the leftovers.
_aix_kill_stale_scripts() {
    # shellcheck disable=SC2086  # AIX_SSH_COMMON_OPTS intentionally word-splits into multiple ssh options
    ssh $AIX_SSH_COMMON_OPTS $AIX_SSH_DEST \
        "for p in \$(ps -ef | grep '[b]ash /tmp/aix_run_cmd_' | awk '{print \$2}'); do kill -TERM -\$(ps -o pgid= -p \$p | tr -d ' ') 2>/dev/null; done" || true
}

# Copy files to/from the AIX host with rsync. Transfers to the AIX cloud LPAR
# occasionally stall with the connection kept alive, so abort after 3 minutes
# of inactivity and retry (up to 3 times) instead of burning the job timeout.
# Future: --partial-dir=.rsync-partial would let retries resume an interrupted
# transfer instead of restarting from scratch.
_aix_transfer() {
    local attempt
    for attempt in 1 2 3; do
        rsync -avz --timeout=180 \
            --rsync-path=/opt/freeware/bin/rsync \
            -e "ssh $AIX_SSH_COMMON_OPTS" \
            "$@" \
        && return 0
        echo "=== AIX: transfer attempt $attempt/3 failed" >&2
        # Back off before retrying: immediate retries tend to land in the same
        # incident window.
        if [ "$attempt" -lt 3 ]; then sleep $((attempt * 15)); fi
    done
    echo "=== AIX: transfer failed after 3 attempts" >&2
    return 1
}

copy_file_from_aix() {
    _aix_transfer "$AIX_SSH_DEST:$1" "$2"
}
copy_file_to_aix() {
    _aix_transfer "$1" "$AIX_SSH_DEST:$2"
}
