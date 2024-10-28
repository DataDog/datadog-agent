#!/bin/bash

set -eou pipefail

NETNS=${2+-n $2}

while [ $(ip $NETNS addr show $1 | grep -c tentative) -ne 0 ];
do
    echo "IPv6 post-up tentative"; sleep 1;
done
