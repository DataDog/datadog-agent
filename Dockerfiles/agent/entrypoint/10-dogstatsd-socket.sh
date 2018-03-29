#!/bin/bash

# Delete the dogstatsd unix socket if present
# FIXME: move that logic to dsd itself
if [[ -e "${DD_DOGSTATSD_SOCKET}" ]]; then
    if [[ -S "${DD_DOGSTATSD_SOCKET}" ]]; then
        echo "Deleting existing socket at ${DD_DOGSTATSD_SOCKET}"
        rm -v "${DD_DOGSTATSD_SOCKET}" || exit $?
    else
        echo "${DD_DOGSTATSD_SOCKET} exists and is not a socket, please check your volume options" >&2
        ls -l "${DD_DOGSTATSD_SOCKET}" >&2
        exit 1
    fi
fi
