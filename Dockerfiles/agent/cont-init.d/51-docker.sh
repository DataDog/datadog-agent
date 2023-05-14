#!/bin/bash
#
# We enable docker if either:
#   - we detect the DOCKER_HOST envvar, overriding the default socket location
#     (in that case, we trust the user wants docker integration and don't check existence)
#   - we find the docker socket at it's default location

if [[ -z "${DOCKER_HOST}" && ! -S /var/run/docker.sock ]]; then
    exit 0
fi


# Set a config for vanilla Docker if no orchestrator was detected
# by the 50-* scripts
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s  /etc/datadog-agent/datadog-docker.yaml \
           /etc/datadog-agent/datadog.yaml
fi
