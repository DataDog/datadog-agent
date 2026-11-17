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
        if new_token=$(dd-octo-sts token --scope DataDog/datadog-agent --policy self.gitlab.read 2>/dev/null) \
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
aix_run_cmd() {
    # Every remote command may fetch from GitHub; make sure the token it
    # gets is not about to expire.
    aix_refresh_github_token
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
    # Disable interactive prompts
    script="export GIT_TERMINAL_PROMPT=0
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
    # shellcheck disable=SC2086  # AIX_SSH_COMMON_OPTS intentionally word-splits into multiple ssh options
    ssh -tt $AIX_SSH_COMMON_OPTS $AIX_SSH_DEST \
        "echo $encoded | openssl enc -base64 -d -A > '$script_file' && bash '$script_file'; rc=\$?; [ \$rc -eq 0 ] && rm -f '$script_file' || echo \"script kept at $script_file for debugging\"; exit \$rc"
}

copy_file_from_aix() {
    # shellcheck disable=SC2086  # AIX_SSH_COMMON_OPTS intentionally word-splits into multiple scp options
    scp $AIX_SSH_COMMON_OPTS "$AIX_SSH_DEST:$1" "$2"
}
copy_file_to_aix() {
    # shellcheck disable=SC2086  # AIX_SSH_COMMON_OPTS intentionally word-splits into multiple scp options
    scp $AIX_SSH_COMMON_OPTS "$1" "$AIX_SSH_DEST:$2"
}
