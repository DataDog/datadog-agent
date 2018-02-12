#!/bin/bash

if [[ -z "${KUBERNETES}" ]]; then
    exit 0
fi

# Set a default config for Kubernetes if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists

cd /etc/datadog-agent/

if [[ ! -e datadog.yaml ]]; then
    ln -s datadog-kubernetes.yaml datadog.yaml
fi

# Enable kubernetes integrations (don't fail if integration absent)

cd /etc/datadog-agent/conf.d

mv kubelet.d/conf.yaml.example kubelet.d/conf.yaml.default || true

# The apiserver check requires leader election to be enabled
if [[ "$DD_LEADER_ELECTION" ]]; then
    mv kubernetes_apiserver.d/conf.yaml.example kubernetes_apiserver.d/conf.yaml.default || true
else
    echo "Disabling the apiserver check as leader election is disabled"
fi

