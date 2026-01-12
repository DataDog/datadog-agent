#!/bin/sh 
set -e

# Absolute path to this script, e.g. /home/user/bin/foo.sh
SCRIPT=$(readlink -f "$0")
# Absolute path this script is in, thus /home/user/bin
SCRIPTPATH=$(dirname "$SCRIPT")

cd "$SCRIPTPATH"

openssl req -x509 -newkey rsa:2048 -keyout example.com.key -out example.com.crt -sha256 -days 400 -nodes -subj "/CN=example.com" \
 -addext "subjectAltName=DNS:example.com"
