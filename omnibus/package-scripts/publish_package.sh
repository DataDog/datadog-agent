#!/bin/bash

TARGET_BUCKET=$1

CODENAME=${2:-$CI_COMMIT_REF_NAME}
TARGET_CODENAME=${CODENAME:-dirty}


if [ -z ${TARGET_BUCKET+x} ]; then
	echo "Missing S3 bucket parameter"
	exit 1;
fi

if [ -z ${STACKSTATE_AGENT_VERSION+x} ]; then
	STACKSTATE_AGENT_VERSION=$(cat $CI_PROJECT_DIR/version.txt)
fi
echo $STACKSTATE_AGENT_VERSION

ls $CI_PROJECT_DIR/outcomes/pkg/*.*

cat <<EOF >~/.gnupg/gpg-agent.conf
default-cache-ttl 46000
allow-preset-passphrase
EOF

gpg-connect-agent RELOADAGENT /bye
echo $SIGNING_PRIVATE_PASSPHRASE | /usr/lib/gnupg2/gpg-preset-passphrase -v -c $(gpg --list-secret-keys --with-fingerprint --with-colons | awk -F: '$1 == "grp" { print $10 }')

deb-s3 upload --sign=${SIGNING_KEY_ID} --codename ${TARGET_CODENAME} --bucket ${TARGET_BUCKET} $CI_PROJECT_DIR/outcomes/pkg/*.deb
