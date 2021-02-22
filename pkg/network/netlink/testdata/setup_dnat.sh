#!/usr/bin/env bash

# required for teardown, so make sure we have it before setup
if ! command -v conntrack >/dev/null 2>&1; then
  echo "conntrack cound not be found. You may need to install conntrack-tools."
  exit 1
fi

set -ex

ip link add dummy1 type dummy
ip address add 1.1.1.1 broadcast + dev dummy1
ip link set dummy1 up
iptables -t nat -A OUTPUT  --dest 2.2.2.2 -j DNAT --to-destination 1.1.1.1
