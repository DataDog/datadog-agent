#!/usr/bin/env bash

set -euo pipefail

printf '=%.0s' {0..79} ; echo

is_cluster_running=$(kind get clusters|grep k8s-e2e-tests||echo none)
if [ "$is_cluster_running" == "k8s-e2e-tests" ]; then
    echo "Cleanup: deleting cluster k8s-e2e-tests"
    kind delete cluster --name k8s-e2e-tests
fi

echo "Setup kind cluster: k8s-e2e-tests"
SCRIPT_DIR=$(dirname "$(readlink -f "$0")")
kind create cluster --name k8s-e2e-tests --wait 10m --config "$SCRIPT_DIR/kind-cluster.yaml"
