#!/usr/bin/env bash

set -euo pipefail

arch=""
case $(uname -m) in
    x86_64)  arch="amd64" ;;
    aarch64) arch="arm64" ;;
    *)
        echo "Unsupported architecture"
        exit 1
        ;;
esac

download_and_install_kubectl() {
    curl --retry 5 --fail --retry-all-errors -LO "https://dl.k8s.io/release/$(curl --retry 5 --fail --retry-all-errors -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$arch/kubectl"
    sudo install kubectl /usr/local/bin/kubectl
}

printf '=%.0s' {0..79} ; echo

if [[ $(uname) == "Darwin" ]]
then
    echo "Kind setup should not be run on Darwin"
    exit 1
fi


# if kubctl is not here, download it
if [[ ! -f ./kubectl ]]; then
    download_and_install_kubectl
else
    # else, download the SHA256 of the wanted version
    curl --retry 5 --fail --retry-all-errors -LO "https://dl.k8s.io/release/$(curl --retry 5 --fail --retry-all-errors -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$arch/kubectl.sha256"
    # And if it differs, force the download again
    if ! echo "$(<kubectl.sha256)  kubectl" | sha256sum --check ; then
        echo "SHA256 of kubectl differs, downloading it again"
        download_and_install_kubectl
    fi
fi

curl -Lo ./kind "https://kind.sigs.k8s.io/dl/v0.21.0/kind-linux-$arch"
sudo install kind /usr/local/bin/kind


if id -u core >/dev/null 2>&1; then
    # skip the usermod step if needless
    sudo usermod -a -G docker core
fi

echo "Kind setup finished"
