#!/usr/bin/env bash
# Sourced by .aix_remote's before_script (see aix_remote.yml for usage).
# Relies on AIX_USER, AIX_HOST, CI_PIPELINE_ID, and CI_JOB_ID being set.

export AIX_SSH_COMMON_OPTS="-o ServerAliveInterval=20 -o ServerAliveCountMax=6"
export AIX_SSH_DEST="$AIX_USER@$AIX_HOST"

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
