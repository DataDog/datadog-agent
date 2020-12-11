#!/usr/bin/env bash

# This script simulates a UDP "client" and "server" using socat for the purposes
# of a system-probe integration test.
# UDP communication within the same process triggers a code path where
# the socket address is not set, and we are unable to instrument "server"
# traffic.

set -ex

if ! command -v socat >/dev/null 2>&1; then
  echo "socat command not be found"
  exit 1
fi

LOCALHOST=127.0.0.1
PORT=8081

# generate messages with fixed sizes which we can look for in tests
SERVER_MESSAGE=$(cat /dev/urandom | base64 | head -c 256)
CLIENT_MESSAGE=$(cat /dev/urandom | base64 | head -c 512)

server() {
	echo -n "$SERVER_MESSAGE" | socat -v stdio udp-listen:"$PORT"
	echo "server done"
}

server &
sleep 1

echo -n "$CLIENT_MESSAGE" | socat -v -t 1 stdio udp:"$LOCALHOST":"$PORT",shut-none
echo "client done"
