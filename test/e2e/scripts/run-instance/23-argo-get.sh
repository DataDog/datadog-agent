#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -o pipefail
set -x

cd "$(dirname $0)"

WORKFLOWS=0
# Wait for any Running workflow
for workflow in $(./argo list -o name)
do
    while ./argo get ${workflow} -o json | jq 'select(.status.phase=="Running")' -re
    do
        sleep 10
    done
    let WORKFLOWS++
done

if [[ "${WORKFLOWS}" == "0" ]]
then
    echo "incorrect workflow number: ${WORKFLOWS}"
    exit 1
fi

set +x
echo "${WORKFLOWS} workflows are not in Running status anymore"

EXIT_CODE=0
for workflow in $(./argo list -o name)
do
    WF=$(./argo get ${workflow} -o json)
    echo "${WF}" | jq 'select(.metadata.labels["workflows.argoproj.io/phase"]=="Succeeded")' -re || {
        # Display the workflow because it didn't match the jq select
        echo "${WF}" | jq .
        EXIT_CODE=2
    }
done

/opt/bin/kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide

exit ${EXIT_CODE}
