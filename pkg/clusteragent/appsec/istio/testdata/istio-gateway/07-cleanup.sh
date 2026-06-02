#!/bin/bash
# Cleanup all resources created by the istio-gateway E2E test
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Cleaning up istio-gateway E2E test resources..."

kubectl delete -f "$DIR/06-second-gateway.yml" 2>/dev/null || true
kubectl delete -f "$DIR/05-envoyfilter.yml" 2>/dev/null || true
kubectl delete -f "$DIR/04-istio-gateway.yml" 2>/dev/null || true
kubectl delete -f "$DIR/03-processor.yml" 2>/dev/null || true
kubectl delete -f "$DIR/02-backend.yml" 2>/dev/null || true
kubectl delete -f "$DIR/01-namespace.yml" 2>/dev/null || true

# Uninstall ingress gateway Helm release if it was installed by setup.sh
if helm status istio-ingressgateway -n istio-system &>/dev/null; then
  helm uninstall istio-ingressgateway -n istio-system
  echo "Istio ingress gateway Helm release uninstalled"
fi

echo "Cleanup complete"
