#!/bin/bash
# Generate traffic to OpenTelemetry demo to ensure active workload

set -e

NAMESPACE="otel-demo"
CONTEXT="kind-gadget-dev"

echo "Generating traffic to OpenTelemetry demo..."

# Get frontend service
FRONTEND_SVC=$(kubectl get svc -n $NAMESPACE frontend --context $CONTEXT -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")

if [ -z "$FRONTEND_SVC" ]; then
    echo "⚠ Frontend service not found, checking frontend-proxy..."
    FRONTEND_SVC=$(kubectl get svc -n $NAMESPACE frontend-proxy --context $CONTEXT -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
fi

if [ -z "$FRONTEND_SVC" ]; then
    echo "❌ No frontend service found in $NAMESPACE"
    exit 1
fi

echo "Found frontend at: $FRONTEND_SVC"

# Create a traffic generator pod
cat <<EOF | kubectl apply --context $CONTEXT -f -
apiVersion: v1
kind: Pod
metadata:
  name: traffic-generator
  namespace: $NAMESPACE
spec:
  restartPolicy: Never
  containers:
  - name: curl
    image: curlimages/curl:latest
    command: ["/bin/sh"]
    args:
    - -c
    - |
      echo "Starting continuous traffic generation..."
      echo "Will run until pod is killed externally"
      i=1
      while true; do
        echo "Request \$i..."
        curl -s -o /dev/null -w "Status: %{http_code}\n" http://${FRONTEND_SVC}:8080/ || echo "Failed"
        sleep 2
        i=\$((i + 1))
      done
EOF

echo "✓ Traffic generator pod created"
echo "  Running continuously (2s interval between requests)"
echo "  Kill with: kubectl delete pod -n $NAMESPACE traffic-generator --context $CONTEXT"
echo ""
echo "Monitor with: kubectl logs -n $NAMESPACE traffic-generator -f --context $CONTEXT"
