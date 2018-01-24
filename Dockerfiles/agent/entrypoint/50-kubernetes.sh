#!/bin/bash

# Set a default config for Kubernetes if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists


if [[ -z "${KUBERNETES}" ]]; then
    exit 0
fi

cd /etc/datadog-agent/

if [[ ! -e datadog.yaml ]]; then
    ln -s datadog-kubernetes.yaml datadog.yaml
fi

mv conf.d/kubernetes_apiserver.d/conf.yaml.example conf.d/kubernetes_apiserver.d/conf.yaml.default
mv conf.d/kubelet.d/conf.yaml.example conf.d/kubelet.d/conf.yaml.default
