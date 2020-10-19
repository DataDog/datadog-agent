#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -o pipefail
set -x

cd "$(dirname $0)"

WORKFLOWS=0
# Wait for any Running workflow
until [[ -z $(./argo list -l workflows.argoproj.io/phase=Running -o name) ]]; do
    sleep 10
done

if [[ -z $(./argo list -o name) ]]; then
    echo "No workflow found"
    exit 1
fi

set +x

EXIT_CODE=0
for workflow in $(./argo list -l workflows.argoproj.io/phase=Failed -o name); do
    ./argo get "$workflow"
    EXIT_CODE=2
done

/opt/bin/kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide

exit ${EXIT_CODE}
