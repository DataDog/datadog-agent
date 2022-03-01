#!/usr/bin/env bash

set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

echo "Cleanup: deleting cluster k8s-e2e-tests"
kind delete cluster --name k8s-e2e-tests
