#!/usr/bin/env bash

set -x

pkill -f "nc -l 0.0.0.0 34567"
pkill -f "nc -l ::0 34568"

ip netns delete test
