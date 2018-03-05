#!/bin/bash

if [[ -z "${KUBERNETES}" ]]; then
    exit 0
fi

# Set a default config for Kubernetes if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists

if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s  /etc/datadog-agent/datadog-kubernetes.yaml \
           /etc/datadog-agent/datadog.yaml
fi

# Enable kubernetes integrations (don't fail if integration absent)

mv /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.example \
   /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.default || true

# The apiserver check requires leader election to be enabled

if [[ "$DD_LEADER_ELECTION" ]]; then
    mv /etc/datadog-agent/conf.d/kubernetes_apiserver.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/kubernetes_apiserver.d/conf.yaml.default || true
else
    echo "Disabling the apiserver check as leader election is disabled"
fi

