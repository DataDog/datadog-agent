#!/bin/bash -e

# NOTICE: this script is meant to be run within a properly configured Docker container,
# do not execute it from your dev environment!

source /etc/profile

PROJECT_NAME=datadog-agent6
LOG_LEVEL=${LOG_LEVEL:-"info"}

export OMNIBUS_SOFTWARE_BRANCH=${OMNIBUS_SOFTWARE_BRANCH:-"master"}
export OMNIBUS_RUBY_BRANCH=${OMNIBUS_RUBY_BRANCH:-"datadog-5.0.0"}

# Clean up omnibus artifacts
rm -rf /var/cache/omnibus/pkg/*

# Clean up what we installed
rm -f /etc/init.d/$PROJECT_NAME
rm -rf /etc/datadog/$PROJECT_NAME
rm -rf /opt/$PROJECT_NAME/*

cd /datadog-agent/omnibus
ls -l

# Install the gems we need, with stubs in bin/
bundle install --without development

omnibus build -l=$LOG_LEVEL $PROJECT_NAME
