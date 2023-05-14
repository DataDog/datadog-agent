#!/bin/bash

if [[ -z "${ECS_FARGATE}" ]]; then
    exit 0
fi

# Set a default config for ECS Fargate if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s  /etc/datadog-agent/datadog-ecs.yaml \
           /etc/datadog-agent/datadog.yaml
fi

# Remove all default checks, AD will automatically enable fargate check
find /etc/datadog-agent/conf.d/ -iname "*.yaml.default" | xargs grep -L 'ad_identifiers' | xargs rm -f
