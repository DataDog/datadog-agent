#!/bin/bash

set -eo xtrace

RETRY_COUNT=$1
ARCH=$2
RUNNER_CMD="$(shift 2; echo "$*")"

KITCHEN_DOCKERS=/kitchen-docker

# Add provisioning steps here !
## Start docker
systemctl start docker
## Load docker images
[ -d $KITCHEN_DOCKERS ] && find $KITCHEN_DOCKERS -maxdepth 1 -type f -exec docker load -i {} \;

# VM provisioning end !

# Start tests
IP=$(ip route get 8.8.8.8 | grep -Po '(?<=(src ))(\S+)')
rm -rf /ci-visibility

CODE=0
/test-runner -retry $RETRY_COUNT $RUNNER_CMD || CODE=$?

pushd /ci-visibility
tar czvf testjson.tar.gz testjson
tar czvf junit.tar.gz junit

exit $CODE
