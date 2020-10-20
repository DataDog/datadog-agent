#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"

for i in {0..60}
do
    /opt/bin/kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide && break
    sleep 5
done

set -e

/opt/bin/kubectl create namespace argo
/opt/bin/kubectl apply -n argo -f https://raw.githubusercontent.com/argoproj/argo/stable/manifests/install.yaml

# TODO use a more restrictive SA
/opt/bin/kubectl apply -f - << EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: argo-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: default
  namespace: default
EOF

set +e

for i in {0..60}
do
    ./argo list && exit 0
    /opt/bin/kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide
    sleep 5
done

exit 1
