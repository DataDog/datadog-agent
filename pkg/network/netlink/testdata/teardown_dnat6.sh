#!/usr/bin/env bash

set -x

# tear down the testing interface, and iptables rule
ip link del dummy0
ip6tables -t nat -D OUTPUT --dest fd::2 -j DNAT --to-destination fd::1

ip -6 r del fd::2 dev eth0

# clear out the conntrack table
conntrack -F
