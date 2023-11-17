#!/bin/bash

if [[ -z "${K_REVISION}" ]]; then
    exit 0
fi

# Set a default config for CloudRun if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s  /etc/datadog-agent/datadog-cloudrun.yaml \
           /etc/datadog-agent/datadog.yaml
fi

# Remove all default checks, AD will automatically enable cloud run check
find /etc/datadog-agent/conf.d/ -iname "*.yaml.default" | xargs grep -L 'ad_identifiers' | xargs rm -f
