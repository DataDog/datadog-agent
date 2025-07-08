#!/bin/bash
set -o errexit
set -o pipefail
set -o nounset
set -o xtrace

# 1. disable kaslr
[ -f $(grep -q "GRUB_CMDLINE_LINUX=\".*nokaslr" /etc/default/grub) ] && sed -i 's/^GRUB_CMDLINE_LINUX="/&nokaslr /' /etc/default/grub
update-grub

apt-key adv --keyserver keyserver.ubuntu.com --recv-keys C8CAB6595FDFF622
codename=$(lsb_release -c | awk  '{print $2}')
tee /etc/apt/sources.list.d/ddebs.list << EOF
deb http://ddebs.ubuntu.com/ ${codename}      main restricted universe multiverse
deb http://ddebs.ubuntu.com/ ${codename}-updates  main restricted universe multiverse
deb http://ddebs.ubuntu.com/ ${codename}-proposed main restricted universe multiverse
EOF

apt update
apt install linux-image-$(uname -r)-dbgsym -y linux-source

cp /usr/lib/debug/boot/vmlinux-`uname -r` /usr/lib/debug/boot/vmlinux.dbg
