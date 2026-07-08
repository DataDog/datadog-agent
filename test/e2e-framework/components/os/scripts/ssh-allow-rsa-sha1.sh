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
#
# PubkeyAcceptedAlgorithms was only introduced in OpenSSH 8.5 (its predecessor,
# PubkeyAcceptedKeyTypes, in 7.0). Appending either directive unconditionally
# would make sshd on older servers (e.g. CentOS 6/7's OpenSSH 5.3/6.x, which
# predate both and don't recognize either name) refuse to start at all,
# knocking out SSH entirely instead of just the RSA/SHA-1 edge case. Servers
# that old only ever spoke the legacy ssh-rsa algorithm anyway, so they need no
# accommodation here in the first place.
SSHD_BIN="$(command -v sshd || echo /usr/sbin/sshd)"
if [ -f /etc/ssh/sshd_config ] && [ -x "$SSHD_BIN" ]; then
  if "$SSHD_BIN" -t -o "PubkeyAcceptedAlgorithms=+ssh-rsa" >/dev/null 2>&1; then
    DIRECTIVE="PubkeyAcceptedAlgorithms"
  elif "$SSHD_BIN" -t -o "PubkeyAcceptedKeyTypes=+ssh-rsa" >/dev/null 2>&1; then
    DIRECTIVE="PubkeyAcceptedKeyTypes"
  else
    DIRECTIVE=""
  fi

  if [ -n "$DIRECTIVE" ]; then
    echo "${DIRECTIVE} +ssh-rsa" >> /etc/ssh/sshd_config
    # Validate the full config before touching the running daemon; if this
    # somehow doesn't pass, leave the existing sshd process untouched rather
    # than restart into a broken config and lose SSH access altogether.
    if "$SSHD_BIN" -t; then
      systemctl restart ssh || systemctl restart sshd || service ssh restart || service sshd restart || true
    fi
  fi
fi
