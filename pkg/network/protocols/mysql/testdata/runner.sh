#!/usr/bin/env bash

# Log the commands and their output for better debugging.
set -x

# Create a directory to store the configuration files
mkdir -p /mysql-test

# Copying the server key and certificate to the test directory, as we change the permissions of the files (required for mysql to run)
cp /certs/* /mysql-test/
chown -R mysql:mysql /mysql-test
chmod 755 /mysql-test
chmod 600 /mysql-test/*

# Call the original entrypoint script
/usr/local/bin/docker-entrypoint.sh "$@"
