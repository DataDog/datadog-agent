#!/bin/bash
set -o errexit
set -o pipefail
set -o nounset
set -o xtrace

# 1. disable kaslr
[ -f $(grep -q "GRUB_CMDLINE_LINUX=\".*nokaslr" /etc/default/grub) ] && sed -i 's/^GRUB_CMDLINE_LINUX="/&nokaslr /' /etc/default/grub
update-grub

# 2. download kernel debug build and kernel sources
apt install -y ubuntu-dbgsym-keyring
echo "Types: deb
URIs: http://ddebs.ubuntu.com/
Suites: $(lsb_release -cs) $(lsb_release -cs)-updates $(lsb_release -cs)-proposed
Components: main restricted universe multiverse
Signed-by: /usr/share/keyrings/ubuntu-dbgsym-keyring.gpg" | tee -a /etc/apt/sources.list.d/ddebs.sources

apt update
apt install -y linux-image-`uname -r`-dbgsym linux-source

cp /usr/lib/debug/boot/vmlinux-`uname -r` /usr/lib/debug/boot/vmlinux.dbg
