#!/usr/bin/env bash

set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

echo "Setup kind cluster: k8s-e2e-tests"

SCRIPT_DIR=$(dirname "$(readlink -f "$0")")
kind create cluster --name k8s-e2e-tests --wait 10m --config "$SCRIPT_DIR/kind-cluster.yaml"
