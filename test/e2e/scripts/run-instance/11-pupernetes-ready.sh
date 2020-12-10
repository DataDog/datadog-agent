#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

journalctl -u pupernetes.service -o cat -e --no-pager

until curl -sf http://127.0.0.1:8989/ready --connect-timeout 1 -w '\n'
do
    sleep 5
done
