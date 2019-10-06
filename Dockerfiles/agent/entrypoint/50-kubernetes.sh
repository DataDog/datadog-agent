#!/bin/bash

if [[ -z "${KUBERNETES}" ]]; then
    exit 0
fi

# Set a default config for Kubernetes if found
# Don't override /etc/stackstate-agent/stackstate.yaml if it exists
if [[ ! -e /etc/stackstate-agent/stackstate.yaml ]]; then
    if [[ -e /var/run/docker.sock ]]; then
        ln -s /etc/stackstate-agent/stackstate-k8s-docker.yaml \
           /etc/stackstate-agent/stackstate.yaml
    else
        ln -s /etc/stackstate-agent/stackstate-kubernetes.yaml \
           /etc/stackstate-agent/stackstate.yaml
    fi
fi

# Enable kubernetes integrations (don't fail if integration absent)
if [[ ! -e /etc/stackstate-agent/conf.d/kubelet.d/conf.yaml.default ]]; then
    mv /etc/stackstate-agent/conf.d/kubelet.d/conf.yaml.example \
    /etc/stackstate-agent/conf.d/kubelet.d/conf.yaml.default
fi

# TODO until we get the kubelet integration ready on https://github.com/StackVista/stackstate-agent-integrations, we safely exit
exit 0
