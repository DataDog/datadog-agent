#!/bin/sh

set -e

if [ -z ${STACKSTATE_AGENT_VERSION+x} ]; then
	# Pick the latest tag by default for our version.
	STACKSTATE_AGENT_VERSION=$(./version.sh)
	# But we will be building from the master branch in this case.
fi

echo $STACKSTATE_AGENT_VERSION

printenv

echo "$SIGNING_PUBLIC_KEY" | gpg --import
echo "$SIGNING_PRIVATE_KEY" | gpg --import
echo "$SIGNING_KEY_ID"

ls $CI_PROJECT_DIR/outcomes/pkg/*.*

debsigs --sign=origin -k ${SIGNING_KEY_ID} $CI_PROJECT_DIR/outcomes/pkg/*.deb
