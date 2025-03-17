#!/bin/sh 
set -e

# ASCII art map of namespaces and interfaces:
#                  |                             |
#    default       |     router                  |     endpoint
#                  |                             |
#        veth0 ------> veth1         veth2 ---------> veth3
#        192.0.2.1 |   192.0.2.2     198.51.100.1|    198.51.100.2
#                  |                             |
#--------------------------------------------------------------------

# create namespaces
ip netns add router
ip netns add endpoint

# create two veth pairs, the router namespace will route veth1 -> veth2
ip link add veth0 type veth peer name veth1
ip link add veth2 type veth peer name veth3

# move interfaces into namespaces
ip link set veth1 netns router
ip link set veth2 netns router
ip link set veth3 netns endpoint

# assign IPs from the TEST-NET-1 CIDR block
ip addr add 192.0.2.1/24 dev veth0
ip link set veth0 up

ip netns exec router ip addr add 192.0.2.2/24 dev veth1
ip netns exec router ip link set veth1 up

# endpoint side has TEST-NET-2 IPs
ip netns exec router ip addr add 198.51.100.1/24 dev veth2
ip netns exec router ip link set veth2 up

ip netns exec router ip link set lo up

ip netns exec endpoint ip addr add 198.51.100.2/24 dev veth3
ip netns exec endpoint ip link set veth3 up

ip netns exec endpoint ip link set lo up

# route the packets inside the router namespace and access the gateway using veth0
ip netns exec router sysctl -w net.ipv4.ip_forward=1
ip netns exec endpoint ip route add default via 198.51.100.1
ip route add 198.51.100.0/24 via 192.0.2.2 dev veth0
