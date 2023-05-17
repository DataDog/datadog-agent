#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"
ssh-keygen -b 4096 -t rsa -C "datadog" -N "" -f "id_rsa"
SSH_RSA=$(cat id_rsa.pub)

arch=$(uname -m)
if [ "$arch" = "arm64" ];
then
  arch="aarch64"
fi

case "$(uname)" in
    Linux)  butane="butane-$arch-unknown-linux-gnu";;
    Darwin) butane="butane-$arch-apple-darwin";;
esac

curl -O     "https://fedoraproject.org/fedora.gpg"
curl -LOC - "https://github.com/coreos/butane/releases/download/v0.16.0/${butane}"
curl -LO    "https://github.com/coreos/butane/releases/download/v0.16.0/${butane}.asc"

gpgv --keyring ./fedora.gpg "${butane}.asc" "$butane"
chmod +x "$butane"

"./$butane" --pretty --strict <<EOF | tee ignition.json
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
        source: "data:,AcceptEnv%20CI%5F%2A%20DD%5FAPI%5FKEY%20DOCKER%5FREGISTRY%5F%2A%20DATADOG%5F%2AAGENT%5FIMAGE%20ARGO%5FWORKFLOW%20DATADOG%5FAGENT%5F%2A%5FKEY%20DATADOG%5FAGENT%5FSITE%0A"
EOF
