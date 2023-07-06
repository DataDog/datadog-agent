#!/bin/bash

set -eo xtrace

# Install dependencies

apt update
apt install -y \
    aria2 \
    fio \
    socat \
    qemu-kvm \
    libvirt-daemon-system \
    curl \
    debootstrap \
    libguestfs-tools \
    libvirt-dev \
    python3-pip \
    nfs-kernel-server \
    rpcbind

if [ "$(uname -m )" == "aarch64" ]; then
    apt install -y qemu-efi-aarch64
fi

systemctl start nfs-kernel-server.service

pip install -r tasks/kernel_matrix_testing/requirements.txt

curl -fsSL https://get.pulumi.com | sh


# Pulumi Setup
pulumi login --local
