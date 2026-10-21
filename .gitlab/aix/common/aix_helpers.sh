#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_USER, AIX_HOST, CI_PIPELINE_ID, and CI_JOB_ID being set.

export AIX_SSH_COMMON_OPTS="-o ServerAliveInterval=20 -o ServerAliveCountMax=6"
export AIX_SSH_DEST="$AIX_USER@$AIX_HOST"

# Non-sensitive CI/GitLab environment variables forwarded to the AIX host, so
# that remote commands run with the pipeline context: eg. packaging scripts
# computing the Agent version with `invoke agent.version` pick up
# CI_PIPELINE_ID and BUCKET_BRANCH to embed the .pipeline.<id> suffix in
# nightly/dev versions, like the deb/rpm/dmg builds do on their runners.
# Keep this list curated on purpose: never forward the whole environment,
# as CI_JOB_TOKEN, Vault secrets and AWS credentials are runner-side only.
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
