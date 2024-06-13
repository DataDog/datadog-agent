#!/bin/sh

# Log the commands and their output for better debugging.
set -x

# Create a directory to store the configuration files
mkdir -p /redis-test

# Copying the server key and certificate to the test directory, as we change the permissions of the files (required for mysql to run)
cp /certs/* /redis-test/
chown -R redis:redis /redis-test
chmod 755 /redis-test
chmod 600 /redis-test/*

# Call the original entrypoint script
/usr/local/bin/docker-entrypoint.sh "$@"
