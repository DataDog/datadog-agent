#!/usr/bin/env bash

set -ex

ip netns add test

ip netns exec test socat STDIO tcp4-listen:34567 &
ip netns exec test socat STDIO tcp6-listen:34568 &
