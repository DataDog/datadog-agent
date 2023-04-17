#!/bin/bash

set -eo xtrace

ARCH=$1
DEPENDENCIES=dependencies-$ARCH.tar.gz

cd /

cp /opt/kernel-version-testing/$DEPENDENCIES /$DEPENDENCIES
tar xzf $DEPENDENCIES --strip-components=1

ls -la /

ls -la system-probe-tests

systemctl start docker

# Add provisioning steps here
eval $(gimme 1.19)

# VM provisioning end

/system-probe-test_spec
