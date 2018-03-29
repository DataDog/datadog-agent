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

# Remove all default cheks & enable fargate check
if [[ ! -e /etc/datadog-agent/conf.d/ecs_fargate.d/conf.yaml.default ]]; then
    find /etc/datadog-agent/conf.d/ -iname "*.yaml.default" -delete
    mv /etc/datadog-agent/conf.d/ecs_fargate.d/conf.yaml.example \
    /etc/datadog-agent/conf.d/ecs_fargate.d/conf.yaml.default
fi
