#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -ex

cd "$(dirname $0)"

./argo install --image-pull-policy IfNotPresent

kubectl apply -f - << EOF
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
    ./argo list && break
    kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide
    sleep 10
done
