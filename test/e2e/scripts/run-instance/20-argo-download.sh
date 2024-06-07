#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo

cd "$(dirname "$0")"

set -e

arch=""
case $(uname -m) in
    x86_64)  arch="amd64" ;;
    aarch64) arch="arm64" ;;
    *)
        echo "Unsupported architecture"
        exit 1
        ;;
esac

# if argo is not here, or if the SHA doesnt match, (re)download it
if [[ ! -f ./argo.gz ]] || ! sha256sum -c "argo.$arch.sha256sum" ; then
    curl -Lf "https://github.com/argoproj/argo-workflows/releases/download/v3.4.3/argo-linux-$arch.gz" -o argo.gz
    # before gunziping it, check its SHA
    if ! sha256sum -c "argo.$arch.sha256sum"; then
        echo "SHA256 of argo.gz differs, exiting."
        exit 1
    fi
fi
if [[ ! -f ./argo. ]]; then
    gunzip -kf argo.gz
fi
chmod +x ./argo
./argo version
