#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
    >&2 echo "usage: $0 [-n <n-attempts>] <command> [arguments]"
    >&2 echo "The script will execute the provided commands and retry in case of failures"
    exit 1
fi

if [ "$1" = "-n" ]; then
    shift
    declare -i NB_ATTEMPTS="$1"
    shift
else
    declare -i NB_ATTEMPTS=5
fi

for i in $(seq 1 $((NB_ATTEMPTS - 1))); do
    "$@" && exit 0
    exitCode=$?
    if [ -n "${NO_RETRY_EXIT_CODE-}" ] && [ "$exitCode" = "$NO_RETRY_EXIT_CODE" ]; then
        >&2 echo "Attempt #$i/$NB_ATTEMPTS failed with error code $exitCode, but not retrying since NO_RETRY_EXIT_CODE is set to $NO_RETRY_EXIT_CODE"
        exit $exitCode
    fi

    >&2 echo "Attempt #$i/$NB_ATTEMPTS failed with error code $exitCode"
    sleep $((i ** 2))
done

"$@"
