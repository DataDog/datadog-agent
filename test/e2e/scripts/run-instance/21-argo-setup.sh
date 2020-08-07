#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname $0)"

for i in {0..60}
do
    kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide && break
    sleep 5
done

set -e

kubectl create ns argo
kubectl apply -n argo -f https://raw.githubusercontent.com/argoproj/argo/master/manifests/install.yaml

set +e

for i in {0..60}
do
    ./argo list && break
    kubectl get hpa,svc,ep,ds,deploy,job,po --all-namespaces -o wide
    sleep 5
done
