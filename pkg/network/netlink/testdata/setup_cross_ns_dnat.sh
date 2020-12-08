#!/usr/bin/env bash

set -ex

ip netns add test
ip link add veth1 type veth peer name veth2
ip link set veth2 netns test
ip address add 2.2.2.3/24 dev veth1
ip -n test address add 2.2.2.4/24 dev veth2
ip link set veth1 up
ip -n test link set veth2 up

# this is required to enable conntrack in the root net namespace
# conntrack won't be enabled unless there is at least one iptables
# rule that uses connection tracking
iptables -I INPUT 1 -m conntrack --ctstate NEW,RELATED,ESTABLISHED -j ACCEPT

ip netns exec test iptables -A PREROUTING -t nat -p tcp --dport 80 -j REDIRECT --to-port 8080
ip netns exec test iptables -A PREROUTING -t nat -p udp --dport 80 -j REDIRECT --to-port 8080
