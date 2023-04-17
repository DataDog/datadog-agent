#!/bin/bash

set -eo xtrace

ARCH=$1
DEPENDENCIES=dependencies-$ARCH.tar.gz

mv /opt/kernel-version-testing/$DEPENDENCIES /$DEPENDENCIES
tar xzvf /$DEPENDENCIES

systemctl start docker

# Add provisioning steps here

# VM provisioning end

go version
