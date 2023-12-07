#!/usr/bin/env bash
set -euo pipefail

function k() {
    # kubectl --kubeconfig XXX --context YYY "$@"
    kubectl "$@"
}

cd "$(dirname "$0")"
k get namespaces                    -o json > namespaces.json
k get pods         --all-namespaces -o json > pods.json
k get services     --all-namespaces -o json > services.json
k get daemonsets   --all-namespaces -o json > daemonsets.json
k get deployments  --all-namespaces -o json > deployments.json
k get statefulsets --all-namespaces -o json > statefulsets.json
k get jobs         --all-namespaces -o json > jobs.json
