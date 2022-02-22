#!/usr/bin/env bash

set -euo pipefail

download_and_install_kubectl() {
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
    sudo install kubectl /usr/local/bin/kubectl
}

printf '=%.0s' {0..79} ; echo
set -x

if [[ $(uname) == "Darwin" ]]
then
    "Kind setup should not be run on Darwin"
    exit 1
fi


# if kubctl is not here, download it
if [[ ! -f ./kubectl ]]; then
    download_and_install_kubectl
else
    # else, download the SHA256 of the wanted version
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl.sha256"
    # And if it differs, force the download again
    if ! echo "$(<kubectl.sha256)  kubectl" | sha256sum --check ; then
        echo "SHA256 of kubectl differs, downloading it again"
        download_and_install_kubectl
    fi
fi

curl -Lo ./kind "https://kind.sigs.k8s.io/dl/$(curl -s -H "Accept: application/vnd.github.v3+json" https://api.github.com/repos/kubernetes-sigs/kind/releases | jq -r '.[0].name')/kind-linux-amd64"
sudo install kind /usr/local/bin/kind


if id -u core >/dev/null 2>&1; then
    # skip the usermod step if needless
    sudo usermod -a -G docker core
fi

echo "Kind setup finished"
