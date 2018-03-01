#!/bin/bash
#
# We enable docker if either:
#   - we detect the DOCKER_HOST envvar, overriding the default socket location
#     (in that case, we trust the user wants docker integration and don't check existence)
#   - we find the docker socket at it's default location

if [[ -z "${DOCKER_HOST}" && ! -e /var/run/docker.sock ]]; then
    exit 0
fi


cd /etc/datadog-agent/

# Set a config for vanilla Docker if no orchestrator was detected
# by the 50-* scripts
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e datadog.yaml ]]; then
    ln -s datadog-docker.yaml datadog.yaml
fi

# Enable the docker corecheck
mv conf.d/docker.d/conf.yaml.example conf.d/docker.d/conf.yaml.default
