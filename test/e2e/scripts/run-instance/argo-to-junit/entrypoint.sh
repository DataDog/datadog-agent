#!/bin/sh
set -e

if [ "$#" -ne 2 ]; then
    /argo_to_junit.py --help
    exit 1
fi

/argo_to_junit.py --input-file $1 --output-file $2
