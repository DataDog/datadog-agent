#!/bin/bash

printf '=%.0s' {0..79} ; echo

for f in /run/systemd/resolve/resolv.conf /etc/resolv.conf /etc/hosts /etc/os-release
do
    echo ${f}
    echo "---"
    cat ${f}
    printf '=%.0s' {0..79} ; echo
done

# pupernetes binary is fetched after instance start-up
echo "waiting for pupernetes binary to be in PATH=${PATH} ..."
for i in {0..60}
do
    which pupernetes 2> /dev/null && break
    sleep 2
done

set -xe
sudo -kE pupernetes wait --unit-to-watch pupernetes.service --logging-since 24h --timeout 15m

kubectl get svc,ep,ds,deploy,job,po --all-namespaces -o wide
