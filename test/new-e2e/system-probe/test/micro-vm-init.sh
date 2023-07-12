#!/bin/bash

set -eo xtrace

GOVERSION=$1
RETRY_COUNT=$2
ARCH=$3
KITCHEN_DOCKERS=/kitchen-docker

# Add provisioning steps here !
## Set go version correctly
eval $(gimme "$GOVERSION")
## Start docker
systemctl start docker
## Load docker images
find $KITCHEN_DOCKERS -maxdepth 1 -type f -exec docker load -i {} \;

# TEMP: bring remanining dependencies
BTFS=btfs-$ARCH.tar.gz
TESTS=tests-$ARCH.tar.gz
cd /
cp /opt/kernel-version-testing/$BTFS /
tar xzvf $BTFS --strip-components=1
cp /opt/kernel-version-testing/$TESTS /
tar xzvf $TESTS --strip-components=1

# VM provisioning end !

# Start tests
IP=$(ip route get 8.8.8.8 | grep -Po '(?<=(src ))(\S+)')
rm -rf /ci-visibility

CODE=0
/test-runner -retry $RETRY_COUNT || CODE=$?

pushd /ci-visibility
tar czvf testjson.tar.gz testjson
tar czvf junit.tar.gz junit

exit $CODE
