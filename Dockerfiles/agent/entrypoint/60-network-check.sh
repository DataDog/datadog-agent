#!/bin/bash

# Disable the host network check if /host/proc is not mounted

if [[ ! -d /host/proc ]]; then
    rm /etc/datadog-agent/conf.d/network.d/conf.yaml.default
fi
