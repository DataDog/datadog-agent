#!/usr/bin/env bash
# Diagnostic-only observer for PR 56549. Never writes Kubernetes resources.
set -uo pipefail
umask 077
out="${E2E_OUTPUT_DIR}/gke-diagnostics"
mkdir -p "$out"
export KUBECONFIG="$(mktemp)"
pods_tmp="$(mktemp)"
trap 'rm -f "$KUBECONFIG" "$pods_tmp"' EXIT
gcloud auth activate-service-account --key-file="$GOOGLE_APPLICATION_CREDENTIALS" --quiet || exit 1
deadline=$((SECONDS + 5400))
while (( SECONDS < deadline )); do
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  # Only clusters belonging to this diagnostic pipeline; the preinitialized
  # runner names them ci-init-<pipeline>-<project>-<suite>.
  if timeout 45s gcloud container clusters list --project=datadog-agent-qa --format=json > "$out/clusters.json"; then
    while IFS=$'\t' read -r cluster location; do
      [ -n "$cluster" ] || continue
      snapshot="$out/$stamp-$cluster"
      mkdir -p "$snapshot"
      if ! timeout 45s gcloud container clusters get-credentials "$cluster" --location="$location" --project=datadog-agent-qa --internal-ip --quiet > "$snapshot/connect.txt" 2>&1; then
        continue
      fi
      kubectl --request-timeout=15s get nodes -o wide > "$snapshot/nodes.txt" 2>&1 || true
      kubectl --request-timeout=15s get events -A --sort-by=.metadata.creationTimestamp > "$snapshot/events.txt" 2>&1 || true
      kubectl --request-timeout=15s get deployments,daemonsets,replicasets -n datadog -o wide > "$snapshot/controllers.txt" 2>&1 || true
      # Keep status and image names, excluding environment variables and secrets.
      if kubectl --request-timeout=15s get pods -A -o json > "$pods_tmp" 2> "$snapshot/pods-error.txt"; then
        jq '[.items[] | {namespace:.metadata.namespace,name:.metadata.name,node:.spec.nodeName,images:([.spec.initContainers[]?.image]+[.spec.containers[]?.image]),status:.status}]' "$pods_tmp" > "$snapshot/pods.json"
        while IFS=$'\t' read -r pod container; do
          kubectl --request-timeout=15s logs -n datadog "$pod" -c "$container" --tail=100 > "$snapshot/$pod-$container.log" 2>&1 || true
          kubectl --request-timeout=15s logs -n datadog "$pod" -c "$container" --previous --tail=100 > "$snapshot/$pod-$container-previous.log" 2>&1 || true
        done < <(jq -r '.items[] | select(.metadata.namespace=="datadog") | .metadata.name as $p | (.spec.initContainers[]?,.spec.containers[]?) | [$p,.name] | @tsv' "$pods_tmp")
      fi
      rm -f "$pods_tmp"
      echo "Captured GKE diagnostics: $snapshot"
    done < <(jq -r --arg prefix "ci-init-${CI_PIPELINE_ID}-" '.[] | select(.name | startswith($prefix)) | [.name,.location] | @tsv' "$out/clusters.json")
  fi
  sleep 20
done
