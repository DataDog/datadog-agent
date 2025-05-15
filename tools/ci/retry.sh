#!/bin/bash

if [ "$#" -lt 1 ]; then
    print "usage: $0 [-n <n-attempts>] <command> [arguments]"
    print "The script will execute the provided commands and retry in case of failures"
    exit 1
fi

if [ "$1" = "-n" ]; then
    shift
    NB_ATTEMPTS="$1"
    shift
else
    NB_ATTEMPTS=5
fi

for i in $(seq 1 $NB_ATTEMPTS); do
    "$@" && exit 0;
    errorcode=$?
    echo "Attempt #${i} failed with error code ${errorcode}"
    # Don't bother sleeping before exiting with an error
    if [ "$i" -lt $NB_ATTEMPTS ]; then
        sleep $((i ** 2))
    fi
done

exit $errorcode
