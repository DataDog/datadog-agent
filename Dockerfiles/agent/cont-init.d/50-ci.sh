#!/bin/bash
set -e

if [[ -z "${DD_INSIDE_CI}" ]]; then
    exit 0
fi

# Set a default config for CI environments
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s  /etc/datadog-agent/datadog-ci.yaml \
           /etc/datadog-agent/datadog.yaml
fi

# Remove all default checks
find /etc/datadog-agent/conf.d/ -iname "*.yaml.default" -delete
