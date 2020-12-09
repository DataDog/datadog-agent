#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

set -ex

socat udp6-listen:53,fork exec:"$DIR/nxdomain_server.py" &
sleep 1
