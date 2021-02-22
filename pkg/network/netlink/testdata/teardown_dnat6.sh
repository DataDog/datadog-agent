#!/usr/bin/env bash

set -x

# tear down the testing interface, and iptables rule
ip link del dummy1
ip6tables -t nat -D OUTPUT --dest fd00::2 -j DNAT --to-destination fd00::1

IFNAME=$(ip route get 8.8.8.8 | awk 'NR == 1 {print $5}')

ip -6 r del fd00::2 dev "${IFNAME}"

# clear out the conntrack table
conntrack -F
