#!/bin/bash

set -e

if [ -z ${STACKSTATE_AGENT_VERSION+x} ]; then
	# Pick the latest tag by default for our version.
	STACKSTATE_AGENT_VERSION=$(inv version -u)
	# But we will be building from the master branch in this case.
fi

echo $STACKSTATE_AGENT_VERSION

printenv

echo "$SIGNING_PUBLIC_KEY" | gpg --import
echo "$SIGNING_PRIVATE_KEY" > gpg_private.key
echo "$SIGNING_PRIVATE_PASSPHRASE" | gpg --batch --yes --passphrase-fd 0 --import gpg_private.key
echo "$SIGNING_KEY_ID"

ls $CI_PROJECT_DIR/outcomes/pkg/*.*

cat <<EOF >~/.gnupg/gpg-agent.conf
default-cache-ttl 46000
allow-preset-passphrase
EOF

gpg-connect-agent RELOADAGENT /bye
echo $SIGNING_PRIVATE_PASSPHRASE | /usr/lib/gnupg2/gpg-preset-passphrase -v -c $(gpg --list-secret-keys --with-fingerprint --with-colons | awk -F: '$1 == "grp" { print $10 }')

debsigs --sign=origin -k ${SIGNING_KEY_ID} $CI_PROJECT_DIR/outcomes/pkg/*.deb
