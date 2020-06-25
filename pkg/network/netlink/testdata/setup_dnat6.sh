#!/usr/bin/env bash

set -ex

ip link add dummy0 type dummy
ip address add fd00::1 dev dummy0
ip link set dummy0 up
ip -6 route add fd00::2 dev eth0
ip6tables -t nat -A OUTPUT --dest fd00::2 -j DNAT --to-destination fd00::1
