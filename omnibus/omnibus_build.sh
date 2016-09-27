#!/bin/bash -e

PROJECT_NAME=datadog-agent6
LOG_LEVEL=${LOG_LEVEL:-"info"}
export OMNIBUS_BRANCH=${OMNIBUS_BRANCH:-"master"}
export OMNIBUS_SOFTWARE_BRANCH=${OMNIBUS_SOFTWARE_BRANCH:-"master"}
export OMNIBUS_RUBY_BRANCH=${OMNIBUS_RUBY_BRANCH:-"datadog-5.0.0"}

# Clean up omnibus artifacts
rm -rf /var/cache/omnibus/pkg/*

# Clean up what we installed
rm -f /etc/init.d/$PROJECT_NAME
rm -rf /etc/datadog/$PROJECT_NAME
rm -rf /opt/datadog/$PROJECT_NAME/*

cd /datadog-agent/omnibus
ls -l

# Install the gems we need, with stubs in bin/
bundle update # Make sure to update to the latest version of omnibus-software
omnibus build -l=$LOG_LEVEL $PROJECT_NAME
