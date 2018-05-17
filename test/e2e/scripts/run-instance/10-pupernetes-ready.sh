#!/bin/bash

set -x

until curl -sf http://127.0.0.1:8989/ready --connect-timeout 1 -w '\n'
do
    for unit in pupernetes p8s-kubelet p8s-etcd
    do
        systemctl status ${unit}.service --no-pager
        echo "---"
    done
    sleep 10
done
