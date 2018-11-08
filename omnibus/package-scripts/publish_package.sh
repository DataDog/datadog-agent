#!/bin/sh

CODENAME=${1:-$CI_COMMIT_REF_NAME}
CURRENT_BRANCH=${CODENAME:-dirty}

if [ -z ${STS_AWS_BUCKET+x} ]; then
	echo "Missing AGENT_S3_BUCKET in environment"
	exit 1;
fi

if [ -z ${STACKSTATE_AGENT_VERSION+x} ]; then
	# Pick the latest tag by default for our version.
	STACKSTATE_AGENT_VERSION=$(./version.sh)
	# But we will be building from the master branch in this case.
fi
echo $STACKSTATE_AGENT_VERSION

ls $CI_PROJECT_DIR/outcomes/pkg/*.*

deb-s3 upload --sign=${SIGNING_KEY_ID} --codename ${CURRENT_BRANCH} --bucket ${STS_AWS_BUCKET:-stackstate-agent-test} $CI_PROJECT_DIR/outcomes/pkg/*.deb
