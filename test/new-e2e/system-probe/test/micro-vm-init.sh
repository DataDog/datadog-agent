#!/bin/bash

set -eo xtrace

GOVERSION=$1
KITCHEN_DOCKERS=/kitchen-docker

# Add provisioning steps here !
## Set go version correctly
eval $(gimme $GOVERSION)
## Start docker
systemctl start docker
## Load docker images
find $KITCHEN_DOCKERS -maxdepth 1 -type f -exec docker load -i {} \;

# VM provisioning end !

# Start tests
/system-probe-test_spec
