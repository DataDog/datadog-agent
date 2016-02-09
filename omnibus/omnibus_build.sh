#!/bin/bash -e

###########################
#
# WARNING: You need to rebuild the docker images if you do any changes to this file
#
############################

PROJECT_DIR=dd-agent-omnibus
PROJECT_NAME=datadog-agent
LOG_LEVEL=${LOG_LEVEL:-"info"}
OMNIBUS_BRANCH=${OMNIBUS_BRANCH:-"master"}
OMNIBUS_SOFTWARE_BRANCH=${OMNIBUS_SOFTWARE_BRANCH:-"master"}

# Clean up omnibus artifacts
rm -rf /var/cache/omnibus/pkg/*

# Clean up what we installed
rm -f /etc/init.d/datadog-agent
rm -rf /etc/dd-agent
rm -rf /opt/$PROJECT_NAME/*

cd $PROJECT_DIR
# Allow to use a different dd-agent-omnibus branch
git fetch --all
git checkout $OMNIBUS_BRANCH
git reset --hard origin/$OMNIBUS_BRANCH

# If an RPM_SIGNING_PASSPHRASE has been passed, let's import the signing key
if [ -n "$RPM_SIGNING_PASSPHRASE" ]; then
  gpg --import /keys/RPM-SIGNING-KEY.private
fi

# Last but not least, let's make sure that we rebuild the agent everytime because
# the extra package files are destroyed when the build container stops (we have
# to tweak omnibus-git-cache directly for that). Same for gohai.
git --git-dir=/var/cache/omnibus/cache/git_cache/opt/datadog-agent tag -d `git --git-dir=/var/cache/omnibus/cache/git_cache/opt/datadog-agent tag -l | grep datadog-agent`
git --git-dir=/var/cache/omnibus/cache/git_cache/opt/datadog-agent tag -d `git --git-dir=/var/cache/omnibus/cache/git_cache/opt/datadog-agent tag -l | grep datadog-gohai`

# Install the gems we need, with stubs in bin/
bundle update # Make sure to update to the latest version of omnibus-software
bin/omnibus build -l=$LOG_LEVEL $PROJECT_NAME
