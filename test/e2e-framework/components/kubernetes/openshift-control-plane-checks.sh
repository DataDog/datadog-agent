#!/bin/bash

# This script is used to check if the OpenShift control plane is ready
export KUBECONFIG=~/.crc/machines/crc/kubeconfig

# API server is responsive
for i in {1..30}; do
  if curl -sk https://127.0.0.1:6443/healthz 2>/dev/null | grep -q ok; then
    echo "API server responsive"
    break
  fi
  echo "Waiting for API server (attempt $i/30)..."
  sleep 10
done

# Nodes are ready
echo "Waiting for nodes to be Ready..."
for i in {1..60}; do
  ready_nodes=$(kubectl get nodes --no-headers 2>/dev/null | grep -c ' Ready ')
  if [ "$ready_nodes" -gt 0 ]; then
    echo "Found $ready_nodes Ready nodes"
    break
  fi
  echo "Waiting for nodes (attempt $i/60)..."
  sleep 5
done

# System pods are running
echo "Waiting for system pods to be running..."
for namespace in openshift-kube-apiserver openshift-kube-controller-manager; do
  for i in {1..60}; do
    running_pods=$(kubectl get pods -n "$namespace" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
    if [ "$running_pods" -gt 0 ]; then
      echo "Namespace $namespace has $running_pods running pod(s)"
      break
    fi
    if [ $i -lt 60 ]; then
      echo "Waiting for $namespace pods (attempt $i/60)..."
      sleep 5
    fi
  done
done

echo "Control plane is ready"
exit 0