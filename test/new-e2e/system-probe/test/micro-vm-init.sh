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
rm -rf ci-visibility

CODE=0
/test-runner -retry 2 || CODE=$?

find /ci-visibility -maxdepth 1 -type d -name testjson-* -exec tar czvf {}-$IP.tar.gz {} \;
find /ci-visibility -maxdepth 1 -type d -name junit-* -exec tar czvf {}-$IP.tar.gz {} \;

exit $CODE
