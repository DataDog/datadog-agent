#!/bin/bash

set -eo xtrace

STACK=$1
USER_ID=$2

DD_AGENT_TESTING_DIR=/root/datadog-agent
ROOT_DIR=kmt-deps
DEPENDENCIES=$ROOT_DIR/$STACK/tests
ARCHIVE_NAME=tests-x86_64.tar.gz
KITCHEN_TESTS=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg

pushd $DD_AGENT_TESTING_DIR/test/new-e2e
GOOS=linux GOARCH=amd64 go build -o test-runner system-probe/test-runner/main.go
GOOS=linux GOARCH=amd64 go build -o test-json-review system-probe/test-json-review/main.go
popd

cp $DD_AGENT_TESTING_DIR/test/new-e2e/test-runner ./
cp $DD_AGENT_TESTING_DIR/test/new-e2e/test-json-review ./

popd

chown -R $UID:$UID $KITCHEN_TESTS

find $DD_AGENT_TESTING_DIR -type d -user root -exec chown -R $USER_ID:$USER_ID {} \;
find $DD_AGENT_TESTING_DIR -type f -user root -exec chown $USER_ID:$USER_ID {} \;
