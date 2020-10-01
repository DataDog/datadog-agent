#!/usr/bin/env bash

set -x

ip netns del test

conntrack -F
