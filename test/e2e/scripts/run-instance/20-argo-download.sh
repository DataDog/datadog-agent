#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"

set -e


# if argo is not here, or if the SHA doesnt match, (re)download it
if [[ ! -f ./argo.gz ]] || ! sha512sum -c argo.sha512sum ; then
    curl -Lf https://github.com/argoproj/argo-workflows/releases/download/v3.1.1/argo-linux-amd64.gz -o argo.gz
    # before gunziping it, check its SHA
    if ! sha512sum -c argo.sha512sum; then
        echo "SHA512 of argo.gz differs, exiting."
        exit 1
    fi
fi
if [[ ! -f ./argo. ]]; then
    gunzip -kf argo.gz
fi
chmod +x ./argo
./argo version
