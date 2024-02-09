#!/bin/bash
#
# SPDX-License-Identifier: BSD-2-Clause

set -eux

if [ $UID -ne 0 ]
then
    # shellcheck disable=SC2086,SC2068
    sudo $0 $@
    exit $?
fi

QUEUE=100
iptables -A OUTPUT -p udp --dport 33434:33634 -d 8.8.8.8 -j NFQUEUE --queue-num "$QUEUE"
# then run ./routest -q $QUEUE -c <your-config.json> -i <interface>
