#!/bin/bash

if [ "$#" -lt 1 ]; then
    print "usage: $0 <command> [arguments]"
    print "The script will execute the provided commands and retry in case of failures"
    exit 1
fi

NB_RETRIES=5

for i in $(seq 1 $NB_RETRIES); do
    "$@" && exit 0;
    errorcode=$?
    # Don't bother sleeping before exiting with an error
    if [ "$i" -lt $NB_RETRIES ]; then
        sleep $((i ** 2))
    fi
done

exit $errorcode
