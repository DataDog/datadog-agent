#!/bin/bash

set -euo pipefail

if [[ $UID == 0 ]] ; then
  echo "Please dont run this script as root, since the gef scripts will get setup for the root user"
  exit 1
fi

echo "[+] apt"
sudo apt-get update
sudo apt-get install -y gdb-multiarch binutils gcc file python3-pip ruby-dev git

echo "[+] pip3"
pip3 install crccheck==1.3.1 unicorn==2.1.4 capstone==5.0.7 ropper==1.13.13 keystone-engine==0.9.2 tqdm==4.67.3

echo "[+] install seccomp-tools, one_gadget"
if [[ -z "$(which seccomp-tools)" ]]; then
    sudo gem install seccomp-tools -v 1.6.2
fi

if [[ -z "$(which one_gadget)" ]]; then
    sudo gem install one_gadget -v 1.10.0
fi

echo "[+] install rp++"
if [[ "$(uname -m)" == "x86_64" ]]; then
    if [[ -z "$(which rp-lin)" ]] && [[ ! -e /usr/local/bin/rp-lin ]]; then
        wget -q https://github.com/0vercl0k/rp/releases/download/v2.1.3/rp-lin-clang.zip -P /tmp
        sudo unzip /tmp/rp-lin-clang.zip -d /usr/local/bin/
        sudo chmod +x /usr/local/bin/rp-lin
        rm /tmp/rp-lin-clang.zip
    fi
fi

echo "[+] install vmlinux-to-elf"
if [[ -z "$(which vmlinux-to-elf)" ]] && [[ ! -e /usr/local/bin/vmlinux-to-elf ]]; then
    pip3 install lz4==4.4.5 zstandard==0.25.0 git+https://github.com/clubby789/python-lzo@b4e39df
    pip3 install vmlinux-to-elf==1.2.3
fi

echo "[+] download gef"
if [[ -e ~/.gdbinit-gef.py ]]; then
    echo "[-] ~/.gdbinit-gef.py already exists. Please delete or rename."
    echo "[-] INSTALLATION FAILED"
    exit 1
else
    # pinned to a specific commit on the dev branch; update by checking https://github.com/bata24/gef/commits/dev
    wget -q https://raw.githubusercontent.com/bata24/gef/c6592a3c9ff9b664313fe1d363158d66d73e7b84/gef.py -O ~/.gdbinit-gef.py
fi

echo "[+] setup gef"
STARTUP_COMMAND="source ~/.gdbinit-gef.py"
if [[ ! -e ~/.gdbinit ]] || [[ -z "$(grep "$STARTUP_COMMAND" ~/.gdbinit)" ]]; then
    echo "$STARTUP_COMMAND" >> ~/.gdbinit
fi

echo "[+] INSTALLATION SUCCESSFUL"
exit 0
