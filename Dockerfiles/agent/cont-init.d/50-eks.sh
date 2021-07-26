#!/bin/bash

if [[ -z "${DD_EKS_FARGATE}" ]]; then
    exit 0
fi

# Set a default config for EKS Fargate if found
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if [[ ! -e /etc/datadog-agent/datadog.yaml ]]; then
    ln -s  /etc/datadog-agent/datadog-kubernetes.yaml \
           /etc/datadog-agent/datadog.yaml
fi

# Remove all default checks & enable kubelet check
if [[ ! -e /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.default ]]; then
    find /etc/datadog-agent/conf.d/ -iname "*.yaml.default" -delete
    mv /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.example \
     /etc/datadog-agent/conf.d/kubelet.d/conf.yaml.default
fi

# Enable eks_fargate check
if [[ ! -e /etc/datadog-agent/conf.d/eks_fargate.d/conf.yaml.default ]]; then
    mv /etc/datadog-agent/conf.d/eks_fargate.d/conf.yaml.example \
     /etc/datadog-agent/conf.d/eks_fargate.d/conf.yaml.default
fi
