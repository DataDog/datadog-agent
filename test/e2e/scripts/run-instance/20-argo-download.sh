#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname $0)"

sha512sum -c argo.sha512sum && {
    chmod +x ./argo
    exit 0
}

set -e
curl -sLf https://github.com/argoproj/argo/releases/download/v2.1.0/argo-linux-amd64 -o argo
sha512sum -c argo.sha512sum
chmod +x ./argo
