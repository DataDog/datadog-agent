#!/bin/bash

if [[ -z "${KUBERNETES}" ]]; then
    exit 0
fi

# Set a default config for Kubernetes if found
# Don't override /etc/stackstate-agent/datadog.yaml if it exists
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

# The apiserver check requires leader election to be enabled
if [[ "$STS_LEADER_ELECTION" ]] && [[ ! -e /etc/stackstate-agent/conf.d/kubernetes_apiserver.d/conf.yaml.default ]]; then
    mv /etc/stackstate-agent/conf.d/kubernetes_apiserver.d/conf.yaml.example \
    /etc/stackstate-agent/conf.d/kubernetes_apiserver.d/conf.yaml.default
else
    echo "Disabling the apiserver check as leader election is disabled"
fi

