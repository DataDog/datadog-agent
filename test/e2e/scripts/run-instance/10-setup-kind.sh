#!/usr/bin/env bash

set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

if [[ $(uname) == "Darwin" ]]
then
    "Kind setup should not be run on Darwin"
    exit 1
fi

KIND_VERSION=$(curl -s -H "Accept: application/vnd.github.v3+json" https://api.github.com/repos/kubernetes-sigs/kind/releases | jq -r '.[0].name')

curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install kubectl /usr/local/bin/kubectl
curl -Lo ./kind https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-linux-amd64
sudo install kind /usr/local/bin/kind

sudo usermod -a -G docker core

echo "Kind setup finished"
