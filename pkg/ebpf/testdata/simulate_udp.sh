#!/usr/bin/env bash

# This script simulates a UDP "client" and "server" using nc for the purposes
# of a system-probe integration test.
# UDP communication within the same process triggers a code path where
# the socket address is not set, and we are unable to instrument "server"
# traffic.

set -ex

LOCALHOST=127.0.0.1
PORT=8081

# generate messages with fixed sizes which we can look for in tests
SERVER_MESSAGE=$(cat /dev/urandom | base64 | head -c 256)
CLIENT_MESSAGE=$(cat /dev/urandom | base64 | head -c 512)

sleep_echo() {
	# sleep for one second, and then echo "$1"
	sleep 1
	echo -n "$1"
}

server() {
	sleep_echo "$SERVER_MESSAGE"  | nc -u -l "$LOCALHOST" "$PORT"
	echo "server done"
}
server &

client() {
	sleep_echo "$CLIENT_MESSAGE" | nc -u  "$LOCALHOST"  "$PORT"
	echo "client done"
}

client &

sleep 2
