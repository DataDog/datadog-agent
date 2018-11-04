#!/bin/bash

# Delete the dogstatsd unix socket if present
# FIXME: move that logic to dsd itself
if [[ -e "${STS_DOGSTATSD_SOCKET}" ]]; then
    if [[ -S "${STS_DOGSTATSD_SOCKET}" ]]; then
        echo "Deleting existing socket at ${STS_DOGSTATSD_SOCKET}"
        rm -v "${STS_DOGSTATSD_SOCKET}" || exit $?
    else
        echo "${STS_DOGSTATSD_SOCKET} exists and is not a socket, please check your volume options" >&2
        ls -l "${STS_DOGSTATSD_SOCKET}" >&2
        exit 1
    fi
fi
