#!/bin/bash

TARGET_BUCKET=$1

CODENAME=${2:-$CI_COMMIT_REF_NAME}
TARGET_CODENAME=${CODENAME:-dirty}


if [ -z ${TARGET_BUCKET+x} ]; then
	echo "Missing S3 bucket parameter"
	exit 1;
fi

if [ -z ${STACKSTATE_AGENT_VERSION+x} ]; then
	# Pick the latest tag by default for our version.
	STACKSTATE_AGENT_VERSION=$(./version.sh)
	# But we will be building from the master branch in this case.
fi
echo $STACKSTATE_AGENT_VERSION

ls $CI_PROJECT_DIR/outcomes/pkg/*.*

deb-s3 upload --sign=${SIGNING_KEY_ID} --codename ${TARGET_CODENAME} --bucket ${TARGET_BUCKET} $CI_PROJECT_DIR/outcomes/pkg/*.deb
