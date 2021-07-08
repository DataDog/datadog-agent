#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"
ssh-keygen -b 4096 -t rsa -C "datadog" -N "" -f "id_rsa"
SSH_RSA=$(cat id_rsa.pub)

case "$(uname)" in
    Linux)  fcct="fcct-$(uname -m)-unknown-linux-gnu";;
    Darwin) fcct="fcct-$(uname -m)-apple-darwin";;
esac
curl -LOC - "https://github.com/coreos/fcct/releases/download/v0.6.0/${fcct}"
curl -LO    "https://github.com/coreos/fcct/releases/download/v0.6.0/${fcct}.asc"
curl https://getfedora.org/static/fedora.gpg | gpg --import
gpg --verify "${fcct}.asc" "$fcct"
chmod +x "$fcct"

"./$fcct" --pretty --strict <<EOF | tee ignition.json
variant: fcos
version: 1.1.0
passwd:
  users:
    - name: core
      ssh_authorized_keys:
        - "${SSH_RSA}"
systemd:
  units:
    - name: zincati.service
      mask: true
    - name: terminate.service
      contents: |
        [Unit]
        Description=Trigger a poweroff

        [Service]
        ExecStart=/bin/systemctl poweroff
        Restart=no
    - name: terminate.timer
      enabled: true
      contents: |
        [Timer]
        OnBootSec=7200

        [Install]
        WantedBy=multi-user.target
storage:
  links:
    - path: /etc/crypto-policies/back-ends/opensshserver.config
      target: /usr/share/crypto-policies/LEGACY/opensshserver.txt
      overwrite: true
  files:
    - path: /etc/ssh/sshd_config.d/99-datadog.conf
      mode: 0400
      contents:
        source: "data:,AcceptEnv%20DOCKER%5FREGISTRY%5F%2A%20DATADOG%5F%2AAGENT%5FIMAGE%0A"
EOF
