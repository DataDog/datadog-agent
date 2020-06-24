#!/usr/bin/env bash

set -ex

ip link add dummy0 type dummy
ip address add fd::1 dev dummy0
ip link set dummy0 up
ip -6 route add fd::2 dev eth0
ip6tables -t nat -A OUTPUT --dest fd::2 -j DNAT --to-destination fd::1
