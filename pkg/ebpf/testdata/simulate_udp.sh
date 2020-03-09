#!/usr/bin/env bash

# This script simulates a UDP "client" and "server" using nc for the purposes
# of a system-probe integration test.
# UDP communication within the same process triggers a code path where
# the socket address is not set, and we are unable to instrument "server"
# traffic.

set -ex

LOCALHOST=127.0.0.1
PORT=8081

sleep_echo() {
	# sleep for one second, and then echo "$1"
	sleep 1
	echo "$1"
}

server() {
	sleep_echo "goodbye world" | nc -u -l "$PORT"
	echo "server done"
}
server &

client() {
	sleep_echo "hello world" | nc -u  "$LOCALHOST"  "$PORT"
	echo "client done"
}

client &

sleep 2
