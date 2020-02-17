#!/usr/bin/env bash

set -ex
ip link add dummy0 type dummy
ip address add  1.1.1.1 broadcast + dev dummy0
ip link set dummy0 up
iptables -t nat -A OUTPUT  --dest 2.2.2.2 -j DNAT --to-destination 1.1.1.1
