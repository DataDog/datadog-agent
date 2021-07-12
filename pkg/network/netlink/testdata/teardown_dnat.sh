#!/usr/bin/env bash

set -x

# tear down the testing interface, and iptables rule
ip link del dummy0
iptables -t nat -D OUTPUT    -d 2.2.2.2  -j DNAT --to-destination 1.1.1.1

# clear out the conntrack table
conntrack -F
