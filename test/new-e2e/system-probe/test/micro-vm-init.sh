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
IP=$(ip -f inet addr show $(ip route get $(getent ahosts google.com | awk '{print $1; exit}') | grep -Po '(?<=(dev ))(\S+)') | sed -En -e 's/.*inet ([0-9.]+).*/\1/p')
rm -f /testjson-$IP.tar.gz
rm -f /junit-$IP.tar.gz

CODE=0
/system-probe-test_spec || CODE=$?

tar czvf /testjson-$IP.tar.gz /testjson
tar czvf /junit-$IP.tar.gz /junit

exit $?
