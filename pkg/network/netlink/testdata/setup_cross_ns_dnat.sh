#!/usr/bin/env bash

set -ex

ip netns add test
ip link add veth1 type veth peer veth2
ip link set veth2 netns test
ip address add 2.2.2.3/24 dev veth1
ip -n test address add 2.2.2.4/24 dev veth2
ip link set veth1 up
ip -n test link set veth2 up

ip netns exec test iptables -A PREROUTING -t nat -p tcp --dport 80 -j REDIRECT --to-port 8080
