#!/bin/bash

if [[ -z "${ECS_FARGATE}" ]]; then
    exit 0
fi

# Set a default config for ECS Fargate if found
# Don't override /etc/stackstate-agent/stackstate.yaml if it exists
if [[ ! -e /etc/stackstate-agent/stackstate.yaml ]]; then
    ln -s  /etc/stackstate-agent/stackstate-ecs.yaml \
           /etc/stackstate-agent/stackstate.yaml
fi

# Remove all default cheks & enable fargate check
if [[ ! -e /etc/stackstate-agent/conf.d/ecs_fargate.d/conf.yaml.default ]]; then
    find /etc/stackstate-agent/conf.d/ -iname "*.yaml.default" -delete
    mv /etc/stackstate-agent/conf.d/ecs_fargate.d/conf.yaml.example \
    /etc/stackstate-agent/conf.d/ecs_fargate.d/conf.yaml.default
fi
