#!/bin/bash
#
# We enable docker if either:
#   - we detect the DOCKER_HOST envvar, overriding the default socket location
#     (in that case, we trust the user wants docker integration and don't check existence)
#   - we find the docker socket at it's default location

if [[ -z "${DOCKER_HOST}" && ! -e /var/run/docker.sock ]]; then
    exit 0
fi


# Set a config for vanilla Docker if no orchestrator was detected
# by the 50-* scripts
# Don't override /etc/stackstate-agent/stackstate.yaml if it exists
if [[ ! -e /etc/stackstate-agent/stackstate.yaml ]]; then
    ln -s  /etc/stackstate-agent/stackstate-docker.yaml \
           /etc/stackstate-agent/stackstate.yaml
fi

# Enable the docker corecheck
# TP: Not using the docker corecheck metrics, provided by the process agent.
#if [[ ! -e /etc/stackstate-agent/conf.d/docker.d/conf.yaml.default ]]; then
#    mv /etc/stackstate-agent/conf.d/docker.d/conf.yaml.example \
#    /etc/stackstate-agent/conf.d/docker.d/conf.yaml.default
#fi
