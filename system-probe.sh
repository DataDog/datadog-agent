#!/bin/bash

die() {
    echo "Error: $1, exiting"
    exit 0
}

RUNTIME_PATH=/opt/datadog-agent/run

if [ ! -d $RUNTIME_PATH ]; then
    sudo mkdir -p $RUNTIME_PATH || die "mkdir $RUNTIME_PATH"
fi

sudo ./system-probe
