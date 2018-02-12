#!/bin/bash

# Set a default config for vanilla Docker
# Don't override /etc/datadog-agent/datadog.yaml if it exists

if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s /etc/datadog-agent/datadog-docker.yaml /etc/datadog-agent/datadog.yaml
fi
