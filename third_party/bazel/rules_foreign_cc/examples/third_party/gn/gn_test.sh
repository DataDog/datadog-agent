#!/usr/bin/env bash

if [[ ! -e "$GN" ]]; then
    echo "gn does not exist"
    exit 1
fi

exec $GN --help
