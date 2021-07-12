#!/usr/bin/env bash

set -x

pkill -f "socat"

ip netns delete test
