#!/bin/bash

# Disable the host network check if /host/proc is not mounted

# It was already done by 50-ecs.sh if the agent is configured for Fargate
if [[ -n "${ECS_FARGATE}" ]]; then
    exit 0
fi

if [[ ! -d /host/proc ]]; then
    rm /etc/datadog-agent/conf.d/network.d/conf.yaml.default
fi
