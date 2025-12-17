#!/usr/bin/env bash

# Quick exit unless ECS managed instances are explicitly enabled
if [[ "${ECS_MANAGED_INSTANCES:-}" != "true" ]]; then
    exit 0
fi

# Require DD_ECS_DEPLOY_MODE=sidecar
if [[ "${DD_ECS_DEPLOYMENT_MODE:-}" != "sidecar" ]]; then
  exit 0
fi

# Set a default config for ECS Managed Instances if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s datadog-ecs.yaml /etc/datadog-agent/datadog.yaml
fi

# Remove all default checks
find /etc/datadog-agent/conf.d -iname '*.yaml.default' -print0 | \
    xargs -0 -r grep -LZ ad_identifiers | \
    xargs -0 -r rm -f
