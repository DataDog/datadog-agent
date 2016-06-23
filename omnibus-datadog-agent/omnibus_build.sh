#!/bin/bash -e

PROJECT_NAME=datadog-agent
LOG_LEVEL=${LOG_LEVEL:-"info"}
export OMNIBUS_BRANCH=${OMNIBUS_BRANCH:-"master"}
export OMNIBUS_SOFTWARE_BRANCH=${OMNIBUS_SOFTWARE_BRANCH:-"master"}
export OMNIBUS_RUBY_BRANCH=${OMNIBUS_RUBY_BRANCH:-"datadog-5.0.0"}

# Clean up omnibus artifacts
rm -rf /var/cache/omnibus/pkg/*

# Clean up what we installed
rm -f /etc/init.d/datadog-agent
rm -rf /etc/dd-agent
rm -rf /opt/$PROJECT_NAME/*

# Install the gems we need, with stubs in bin/
gem install update
bundle update # Make sure to update to the latest version of omnibus-software
bin/omnibus build -l=$LOG_LEVEL $PROJECT_NAME
