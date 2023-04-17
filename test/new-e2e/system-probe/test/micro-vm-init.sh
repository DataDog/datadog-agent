#!/bin/bash

set -eo xtrace

ARCH=$1
DEPENDENCIES=dependencies-$ARCH.tar.gz

cd /

cp /opt/kernel-version-testing/$DEPENDENCIES /$DEPENDENCIES
tar xzvf $DEPENDENCIES --strip-components=1

ls -la /

ls -la system-probe-tests

systemctl start docker

# Add provisioning steps here

# VM provisioning end

go version
