#!/usr/bin/env bash


cd $(dirname $0)

set -ex

ssh-keygen -b 4096 -t rsa -C "datadog" -N "" -f "id_rsa"
SSH_RSA=$(cat id_rsa.pub)

cat << EOF | tee ignition.json
{
  "passwd": {
    "users": [
      {
        "sshAuthorizedKeys": [
          "${SSH_RSA}"
        ],
        "name": "core"
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "mask": true,
        "name": "user-configdrive.service"
      },
      {
        "mask": true,
        "name": "user-configvirtfs.service"
      },
      {
        "enabled": true,
        "name": "oem-cloudinit.service",
        "contents": "[Unit]\nDescription=Cloudinit from platform metadata\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/coreos-cloudinit --oem=ec2-compat\n\n[Install]\nWantedBy=multi-user.target\n"
      },
      {
        "enabled": true,
        "name": "setup-pupernetes.service",
        "contents": "[Unit]\nDescription=Setup pupernetes\n\n[Service]\nType=oneshot\nExecStart=/opt/bin/setup-pupernetes\nRemainAfterExit=yes\n\n[Install]\nWantedBy=multi-user.target\n"
      },
      {
        "enabled": true,
        "name": "pupernetes.service",
        "contents": "[Unit]\nDescription=Run pupernetes\nRequires=setup-pupernetes.service docker.service\nAfter=setup-pupernetes.service docker.service\n\n[Service]\nEnvironment=SUDO_USER=core\nExecStart=/opt/bin/pupernetes run /opt/sandbox -v 5 --kubectl-link /opt/bin/kubectl\nRestart=on-failure\nRestartSec=10\n\n[Install]\nWantedBy=multi-user.target\n"
      }
    ]
  },
  "storage": {
    "files": [
      {
        "path": "/opt/bin/setup-pupernetes",
        "mode": 320,
        "contents": {
          "source": "data:,%23%21%2Fbin%2Fbash%20-ex%0A%0Acd%20%2Fopt%2Fbin%0A%0Acurl%20-Lf%20https%3A%2F%2Fs3.us-east-2.amazonaws.com%2Fpupernetes%2Flatest%2Fpupernetes%20-o%20%2Fopt%2Fbin%2Fpupernetes%0Acurl%20-Lf%20https%3A%2F%2Fs3.us-east-2.amazonaws.com%2Fpupernetes%2Flatest%2Fpupernetes.sha512sum%20-o%20%2Fopt%2Fbin%2Fpupernetes.sha512sum%0A%0Asha512sum%20-c%20%2Fopt%2Fbin%2Fpupernetes.sha512sum%0A%0Achmod%20%2Bx%20%2Fopt%2Fbin%2Fpupernetes%0A"
        },
        "filesystem": "root"
      },
      {
        "path": "/home/core/.kube/config",
        "filesystem": "root",
        "user": {
          "name": "core"
        },
        "mode": 420
      }
    ]
  },
  "ignition": {
    "version": "2.1.0"
  }
}
EOF
