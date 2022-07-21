#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

ARGO_WORKFLOW=${ARGO_WORKFLOW:-''}

cd "$(dirname "$0")"

# Wait for any Running workflow
until [[ "$(./argo list --running -o name)" == "No workflows found" ]]; do
    sleep 10
done

if [[ "$(./argo list -o name)" == "No workflows found" ]]; then
    echo "No workflow found"
    exit 1
fi

set +x

if ! locale -k LC_CTYPE | grep -qi 'charmap="utf-\+8"'; then
    no_utf8_opt='--no-utf8'
fi

for workflow in $(./argo list --status Succeeded -o name | grep -v 'No workflows found'); do
    ./argo get ${no_utf8_opt:-} "$workflow"
done

EXIT_CODE=0
for workflow in $(./argo list --status Failed -o name | grep -v 'No workflows found'); do
    ./argo get "$workflow" -o json | jq -r '.status.nodes | to_entries | map(.value) | sort_by(.phase) | .[] | select(.phase == "Failed" or (.name | contains("onExit[0].diagnose[0]."))) | .displayName + " " + .id' | while read -r displayName podName; do
        printf '\033[1m===> Logs of %s\t%s <===\033[0m\n' "$displayName" "$podName"
        ./argo logs "$workflow" "$podName"
    done
    ./argo get ${no_utf8_opt:-} "$workflow"
    EXIT_CODE=2
done

# CWS & CSPM e2e output
for workflow in $(./argo list -o name); do
    if [ "$ARGO_WORKFLOW" = "cws" ]; then
        pod=$(./argo get "$workflow" -o json | jq -r '.status.nodes[] | select(.displayName=="test-cws-e2e").id')
        if [ -n "$pod" ]; then
            kubectl logs "$pod" -c main
        fi
    fi

    if [ "$ARGO_WORKFLOW" = "cspm" ]; then
        pod=$(./argo get "$workflow" -o json | jq -r '.status.nodes[] | select(.displayName=="test-cspm-e2e").id')
        if [ -n "$pod" ]; then
            kubectl logs "$pod" -c main
        fi
    fi
done

# Make the Argo UI available from the user
kubectl --namespace argo patch service/argo-server --type json --patch $'[{"op": "replace", "path": "/spec/type", "value": "NodePort"}, {"op": "replace", "path": "/spec/ports", "value": [{"port": 2746, "nodePort": 30001, "targetPort": 2746}]}]'

# In case of failure, let's keep the VM for 1 day instead of 2 hours for investigation
if [[ $EXIT_CODE != 0 ]]; then
    sudo sed -i 's/^OnBootSec=.*/OnBootSec=86400/' /etc/systemd/system/terminate.timer
    sudo systemctl daemon-reload
    sudo systemctl restart terminate.timer
fi

TIME_LEFT=$(systemctl status terminate.timer | awk '$1 == "Trigger:" {print gensub(/ *Trigger: (.*)/, "\\1", 1)}')
LOCAL_IP=$(curl -s http://169.254.169.254/2020-10-27/meta-data/local-ipv4)

printf "\033[1mThe Argo UI will remain available at \033[1;34mhttps://%s\033[0m until \033[1;33m%s\033[0m.\n" "$LOCAL_IP" "$TIME_LEFT"
printf "\033[1mAll the logs of this job can be found at \033[1;34mhttps://dddev.datadoghq.com/logs?query=app%%3Aagent-e2e-tests%%20ci_commit_short_sha%%3A%s%%20ci_pipeline_id%%3A%s%%20ci_job_id%%3A%s&index=&from_ts=%d000&to_ts=%d000\033[0m.\n" "${CI_COMMIT_SHORT_SHA:-unknown}" "${CI_PIPELINE_ID:-unknown}" "${CI_JOB_ID:-unknown}" "$(date -d '2 hours ago' +%s)" "$(date +%s)"

exit ${EXIT_CODE}
