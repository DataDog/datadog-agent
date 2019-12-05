#!/bin/bash

if [[ -z "${KUBERNETES}" ]]; then
    exit 0
fi

# Set a default config for Kubernetes if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    if [[ -e /var/run/docker.sock ]]; then
        ln -s /etc/datadog-agent/datadog-k8s-docker.yaml \
           /etc/datadog-agent/datadog.yaml
    else
        ln -s /etc/datadog-agent/datadog-kubernetes.yaml \
           /etc/datadog-agent/datadog.yaml
    fi
fi

# Enable kubernetes integrations (don't fail if integration absent)
if [[ ! -e /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.default ]]; then
    mv /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.example \
    /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.default
fi

# The apiserver check requires leader election to be enabled
if [[ "$DD_LEADER_ELECTION" == "true" ]] && [[ ! -e /etc/datadog-agent/conf.d/kubernetes_apiserver.d/conf.yaml.default ]]; then
    mv /etc/datadog-agent/conf.d/kubernetes_apiserver.d/conf.yaml.example \
    /etc/datadog-agent/conf.d/kubernetes_apiserver.d/conf.yaml.default
else
    echo "Disabling the apiserver check as leader election is disabled"
fi

