#!/usr/bin/env bash

set -x

ip link del veth1
ip -n test link del veth2
ip netns del test

conntrack -F
