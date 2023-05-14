#!/usr/bin/env bash
set -euo pipefail

readonly ENTRYPOINT_PATH="/opt/entrypoints"

if [[ -z ${ENTRYPOINT+x} ]]; then
    # Default entrypoint if none is specified
    if [[ -x "$ENTRYPOINT_PATH/_default" ]]; then
        exec "$ENTRYPOINT_PATH/_default"
    else
        printf "No entrypoint is defined.\n" >&2
        exit 1
    fi
fi

# Prevent directory traversal attacks
if [[ "${ENTRYPOINT/\//_}" != "$ENTRYPOINT" ]]; then
    printf "\"%s\" is not a valid ENTRYPOINT.\n" "$ENTRYPOINT" >&2
    exit 1
fi

if [[ -x "$ENTRYPOINT_PATH/$ENTRYPOINT" ]]; then
    exec "$ENTRYPOINT_PATH/$ENTRYPOINT"
else
    printf "\"%s\" is not a valid ENTRYPOINT.\n" "$ENTRYPOINT" >&2
    exit 1
fi
