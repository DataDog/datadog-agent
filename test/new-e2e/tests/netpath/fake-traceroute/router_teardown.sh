#!/bin/sh
# no set -e because these may or may not exist
ip link delete veth0
ip link delete veth1
ip link delete veth2
ip link delete veth3
ip netns delete router
ip netns delete endpoint
