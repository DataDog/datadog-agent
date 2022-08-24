#!/usr/bin/env bash

set -ex

if ! command -v socat >/dev/null 2>&1; then
  echo "socat cound not be found"
  exit 1
fi

ip netns exec "$1" socat STDIO tcp4-listen:34567 &
ip netns exec "$1" socat STDIO tcp6-listen:34568 &
ip netns exec "$1" socat STDIO udp4-listen:34567 &
ip netns exec "$1" socat STDIO udp6-listen:34568 &
