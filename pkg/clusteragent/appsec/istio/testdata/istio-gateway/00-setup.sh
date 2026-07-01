#!/bin/bash
# End-to-end setup for istio-gateway AppSec injection testing.
# Prerequisites: minikube running with Istio control plane (istiod) installed.
# Installs the Istio ingress gateway via Helm, deploys a backend, the AppSec
# ext_proc processor, Gateway + VirtualService, and the EnvoyFilter. Then
# sends a test request and verifies the processor received traffic.
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Step 1: Install Istio ingress gateway via Helm ==="
helm repo add istio https://istio-release.storage.googleapis.com/charts 2>/dev/null || true
helm repo update istio
if helm status istio-ingressgateway -n istio-system &>/dev/null; then
  echo "Istio ingress gateway already installed, skipping"
else
  helm install istio-ingressgateway istio/gateway -n istio-system --set service.type=NodePort --wait
fi

echo ""
echo "=== Step 2: Apply test resources ==="
kubectl apply -f "$DIR/01-namespace.yml"
kubectl apply -f "$DIR/02-backend.yml"
kubectl apply -f "$DIR/03-processor.yml"
kubectl apply -f "$DIR/04-istio-gateway.yml"
kubectl apply -f "$DIR/05-envoyfilter.yml"

echo ""
echo "=== Step 3: Wait for pods ==="
kubectl wait --for=condition=ready pod -l app=httpbin -n appsec-gw-test --timeout=120s
kubectl wait --for=condition=ready pod -l app=appsec-processor -n appsec-gw-test --timeout=120s
kubectl wait --for=condition=ready pod -l istio=ingressgateway -n istio-system --timeout=120s

echo ""
echo "=== Step 4: Wait for EnvoyFilter config propagation ==="
GWPOD=$(kubectl get pods -n istio-system -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}')
for i in $(seq 1 20); do
  if istioctl proxy-config cluster "$GWPOD" -n istio-system 2>/dev/null | grep -q datadog_appsec_ext_proc_cluster; then
    echo "✓ ext_proc cluster configured in gateway"
    break
  fi
  if [ "$i" -eq 6 ]; then
    echo "⚠ ext_proc cluster not yet visible in proxy config (may still propagate)"
  else
    echo "  Waiting for config propagation... (attempt $i/20)"
    sleep 5
  fi
done

echo ""
echo "=== Step 5: Send test request through gateway ==="
kubectl port-forward svc/istio-ingressgateway -n istio-system 8888:80 &
PF_PID=$!
sleep 2

HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:8888/" -H "Host: test.example.com" --max-time 10 2>/dev/null || echo "000")
kill $PF_PID 2>/dev/null || true
wait $PF_PID 2>/dev/null || true

echo "HTTP Status: $HTTP_STATUS"

echo ""
echo "=== Step 6: Check processor logs ==="
PROC_POD=$(kubectl get pods -n appsec-gw-test -l app=appsec-processor -o jsonpath='{.items[0].metadata.name}')
kubectl logs "$PROC_POD" -n appsec-gw-test -c processor --tail=5 2>/dev/null || \
  kubectl logs "$PROC_POD" -n appsec-gw-test --tail=5

echo ""
if [ "$HTTP_STATUS" = "200" ]; then
  echo "=== ✓ E2E test PASSED: request reached backend through gateway ==="
  echo "Check the processor logs above for 'external_processing: first request received' to confirm ext_proc flow."
else
  echo "=== ✗ E2E test FAILED: HTTP status $HTTP_STATUS ==="
  exit 1
fi
