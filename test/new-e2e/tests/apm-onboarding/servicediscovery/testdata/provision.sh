#!/usr/bin/env bash

set -e

apt-get update
apt-get install -y ca-certificates curl gnupg python3

# Install Node
if [ ! -f "${HOME}/.nvm/nvm.sh" ]; then
    curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
fi

source "${HOME}/.nvm/nvm.sh"
nvm install 20

# Install Go
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
rm go1.21.0.linux-amd64.tar.gz
