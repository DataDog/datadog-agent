#!/bin/bash

if [[ -z "${ECS_FARGATE}" ]]; then
    exit 0
fi

# Set a default config for ECS Fargate if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists

cd /etc/datadog-agent/

if [[ ! -e datadog.yaml ]]; then
    ln -s datadog-ecs.yaml datadog.yaml
fi

# Remove all default checks (no host)

find conf.d/ -iname "*.yaml.default" -delete
