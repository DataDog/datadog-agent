#!/bin/bash

# Set a fallback empty config with no AD
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    touch  /etc/datadog-agent/datadog.yaml
fi
