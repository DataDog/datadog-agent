#!/usr/bin/env bash

set -ex

# required for teardown, so make sure we have it before setup
if ! command -v conntrack >/dev/null 2>&1; then
  echo "conntrack cound not be found. You may need to install conntrack-tools."
  exit 1
fi

IFNAME=$(ip route get 8.8.8.8 | awk 'NR == 1 {print $5}')

ip link add dummy1 type dummy
ip address add fd00::1 dev dummy1
ip link set dummy1 up
ip -6 route add fd00::2 dev "${IFNAME}"
ip6tables -t nat -A OUTPUT --dest fd00::2 -j DNAT --to-destination fd00::1
