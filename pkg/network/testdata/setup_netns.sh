#!/usr/bin/env bash

set -ex

ip netns add test

ip netns exec test nc -l 0.0.0.0 34567 &
ip netns exec test nc -l ::0 34568 &
