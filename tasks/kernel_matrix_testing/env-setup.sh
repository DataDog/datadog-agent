#!/bin/bash

set -eo xtrace

DIR_NAME=$(dirname "$0")

# Install dependencies

sudo apt update
sudo apt install -y \
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
    rpcbind \
    ssh-askpass \
    xsltproc

if [ "$(uname -m )" == "aarch64" ]; then
    sudo apt install -y qemu-efi-aarch64
fi

sudo systemctl start nfs-kernel-server.service

pip3 install -r "${DIR_NAME}"/requirements.txt

curl -fsSL https://get.pulumi.com | sh


# Pulumi Setup
source ~/.bashrc
pulumi login --local
