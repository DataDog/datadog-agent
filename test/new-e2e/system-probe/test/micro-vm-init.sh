#!/bin/bash

set -eo xtrace

GOVERSION=$1
KITCHEN_DOCKERS=/kitchen-docker

# Add provisioning steps here !
## Set go version correctly
eval $(gimme "$GOVERSION")
## Start docker
systemctl start docker
## Load docker images
find $KITCHEN_DOCKERS -maxdepth 1 -type f -exec docker load -i {} \;

# VM provisioning end !

# Start tests
IP=$(ip route get 8.8.8.8 | grep -Po '(?<=(src ))(\S+)')
rm -f "/testjson-$IP.tar.gz"
rm -f "/junit-$IP.tar.gz"

CODE=0
/system-probe-test_spec || CODE=$?

tar czvf "/testjson-$IP.tar.gz" /testjson
tar czvf "/junit-$IP.tar.gz" /junit

exit $CODE
