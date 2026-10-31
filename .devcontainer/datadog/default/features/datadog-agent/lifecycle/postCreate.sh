#!/bin/bash

# Match bits' supplementary group to the host Docker socket GID. The socket is
# bind-mounted at runtime, so its GID cannot be determined while building the image.
if [[ -S /var/run/docker.sock ]]; then
    socket_gid=$(stat -c '%g' /var/run/docker.sock)
    socket_group=$(getent group "$socket_gid" | cut -d: -f1 || true)
    if [[ -z "$socket_group" ]]; then
        socket_group=dockersock
        sudo groupadd --gid "$socket_gid" "$socket_group"
    fi
    if ! id -nG bits | tr ' ' '\n' | grep -qx "$socket_group"; then
        sudo usermod --append --groups "$socket_group" bits
    fi
fi

# Run install tools
cd ~/dd/datadog-agent

# Tweaking environment variables, will be removed once the Docker image is updated.
# Make sure we can dynamically install dda dependencies
export DDA_NO_DYNAMIC_DEPS=0
# Unset GOPATH otherwise it is set to /go in the build image base
unset GOPATH

dda inv install-tools 2>&1 | tee "/home/bits/.install-tools.log"
dda inv vscode.setup 2>&1 | tee "/home/bits/.vscode-setup.log"
