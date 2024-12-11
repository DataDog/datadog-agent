#!/bin/bash

handler() {
    exit 0
}
trap 'handler' SIGINT SIGHUP SIGQUIT SIGTERM

while true; do
    sleep 1
done
