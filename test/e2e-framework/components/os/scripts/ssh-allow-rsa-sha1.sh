#!/bin/bash

set -e

# TODO(agent-devx-loops): remove once every CI job connecting to e2e VMs (deb_x64
# image) ships an OpenSSH client new enough to negotiate rsa-sha2-256/rsa-sha2-512.
#
# Some CI images still ship an OpenSSH client old enough (pre-7.2, e.g. Ubuntu
# 14.04's 6.6p1) to only offer the deprecated SHA-1 ssh-rsa signature algorithm
# when authenticating with an RSA key. OpenSSH servers 8.8+ disable ssh-rsa by
# default, which rejects those clients outright with "Permission denied
# (publickey)" even though the key itself is valid. Re-allow it on these
# short-lived test VMs so old and new clients can both connect.
if [ -f /etc/ssh/sshd_config ]; then
  echo "PubkeyAcceptedAlgorithms +ssh-rsa" >> /etc/ssh/sshd_config
  systemctl restart ssh || systemctl restart sshd || service ssh restart || service sshd restart
fi
