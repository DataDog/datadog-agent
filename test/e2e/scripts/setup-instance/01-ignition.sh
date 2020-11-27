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
    - name: setup-pupernetes.service
      enabled: true
      contents: |
        [Unit]
        Description=Setup pupernetes
        Wants=network-online.target
        After=network-online.target

        [Service]
        Type=oneshot
        ExecStart=/usr/local/bin/setup-pupernetes
        RemainAfterExit=yes

        [Install]
        WantedBy=multi-user.target
    - name: install-pupernetes-dependencies.service
      enabled: true
      contents: |
        [Unit]
        Description=Install pupernetes dependencies
        Wants=network-online.target
        After=network-online.target

        [Service]
        Type=oneshot
        ExecStart=/usr/bin/rpm-ostree install --idempotent --reboot unzip
        RemainAfterExit=yes

        [Install]
        WantedBy=multi-user.target
    - name: pupernetes.service
      enabled: true
      contents: |
        [Unit]
        Description=Run pupernetes
        Requires=setup-pupernetes.service install-pupernetes-dependencies.service docker.service
        After=setup-pupernetes.service install-pupernetes-dependencies.service docker.service

        [Service]
        Environment=SUDO_USER=core
        WorkingDirectory=/home/core
        ExecStartPre=/usr/bin/mkdir -p /opt/bin
        ExecStartPre=/usr/sbin/setenforce 0
        ExecStartPre=-/usr/bin/rpm-ostree usroverlay
        ExecStart=/usr/local/bin/pupernetes daemon run /opt/sandbox --kubectl-link /opt/bin/kubectl -v 5 --hyperkube-version 1.18.2 --run-timeout 6h
        Restart=on-failure
        RestartSec=5
        Type=notify
        TimeoutStartSec=600
        TimeoutStopSec=120

        [Install]
        WantedBy=multi-user.target
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
  files:
    - path: /usr/local/bin/setup-pupernetes
      mode: 0500
      contents:
        source: "data:,%23%21%2Fbin%2Fbash%20-ex%0Acurl%20-Lf%20--retry%207%20--retry-connrefused%20https%3A%2F%2Fgithub.com%2FDataDog%2Fpupernetes%2Freleases%2Fdownload%2Fv0.12.0%2Fpupernetes%20-o%20%2Fusr%2Flocal%2Fbin%2Fpupernetes%0Asha512sum%20-c%20%2Fusr%2Flocal%2Fshare%2Fpupernetes.sha512sum%0Achmod%20%2Bx%20%2Fusr%2Flocal%2Fbin%2Fpupernetes%0A"
    - path: /usr/local/share/pupernetes.sha512sum
      mode: 0400
      contents:
        source: "data:,c0cd502d7dc8112e4c17e267068a12f150d334e1eca7e831130e462f5a431d044b10019af8533b756ee6d10a3fd4e9c72a62cee6d6a0045caa57807d06ede817%20%2Fusr%2Flocal%2Fbin%2Fpupernetes%0A"
    - path: /etc/ssh/sshd_config.d/99-datadog.conf
      mode: 0400
      contents:
        source: "data:,AcceptEnv%20DOCKER%5FREGISTRY%5F%2A%20DATADOG%5F%2AAGENT%5FIMAGE%0A"
EOF
