#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -x

cat /run/systemd/resolve/resolv.conf
cat /etc/resolv.conf
cat /etc/hosts

until curl -sf http://127.0.0.1:8989/ready --connect-timeout 1 -w '\n'
do
    systemctl status pupernetes.service --no-pager --full
    journalctl -u pupernetes.service --no-pager -o cat -n 50 -e

    sleep 10
done
