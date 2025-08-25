#!/bin/bash
set -o errexit
set -o pipefail
set -o nounset

# 1. disable kaslr
[[ ! $(grep -q "GRUB_CMDLINE_LINUX=\".*nokaslr" /etc/default/grub) ]] && sed -i 's/^GRUB_CMDLINE_LINUX="/&nokaslr /' /etc/default/grub
update-grub

# 2. download kernel debug build and kernel sources
codename=$(lsb_release -c | awk  '{print $2}')
if [[ "${codename}" == "xenial" ]]; then
    apt-key adv --keyserver keyserver.ubuntu.com --recv-keys C8CAB6595FDFF622
    tee /etc/apt/sources.list.d/ddebs.list << EOF
    deb http://ddebs.ubuntu.com/ ${codename}      main restricted universe multiverse
    deb http://ddebs.ubuntu.com/ ${codename}-updates  main restricted universe multiverse
    deb http://ddebs.ubuntu.com/ ${codename}-proposed main restricted universe multiverse
EOF
else
    apt install -y ubuntu-dbgsym-keyring
    echo "\
Types: deb
URIs: http://ddebs.ubuntu.com/
Suites: $(lsb_release -cs) $(lsb_release -cs)-updates $(lsb_release -cs)-proposed
Components: main restricted universe multiverse
Signed-by: /usr/share/keyrings/ubuntu-dbgsym-keyring.gpg" | tee -a /etc/apt/sources.list.d/ddebs.sources
fi

apt update
apt install -y linux-image-`uname -r`-dbgsym linux-source

cp /usr/lib/debug/boot/vmlinux-`uname -r` /usr/lib/debug/boot/vmlinux.dbg
