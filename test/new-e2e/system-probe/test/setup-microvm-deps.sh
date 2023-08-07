#!/bin/bash

set -eo xtrace

STACK=$1
USER_ID=$2

DD_AGENT_TESTING_DIR=/root/datadog-agent
ROOT_DIR=kmt-deps
DEPENDENCIES=$ROOT_DIR/$STACK/dependencies
ARCHIVE_NAME=dependencies-x86_64.tar.gz
CLANG_BPF=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/clang-bpf
LLC_BPF=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/llc-bpf
GO_BIN=go/bin
GOTESTSUM=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/gotestsum
TEST2JSON=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/test2json
EMBEDDED_BIN=opt/datadog-agent/embedded/bin
EMBEDDED_INC=opt/datadog-agent/embedded/include
SYSTEM_PROBE_TESTS=/opt/system-probe-tests

[ -f $TEST2JSON ] || cp $(go env GOTOOLDIR)/test2json $TEST2JSON

rm -rf $DEPENDENCIES
mkdir -p $DEPENDENCIES
pushd $DEPENDENCIES
mkdir -p $EMBEDDED_BIN
mkdir -p $SYSTEM_PROBE_TESTS
cp $CLANG_BPF $EMBEDDED_BIN
cp $LLC_BPF $EMBEDDED_BIN
mkdir -p $EMBEDDED_INC
mkdir -p $GO_BIN
cp $GOTESTSUM $GO_BIN
cp $TEST2JSON $GO_BIN
mkdir junit
mkdir testjson
mkdir pkgjson
cp $DD_AGENT_TESTING_DIR/test/new-e2e/system-probe/test/micro-vm-init.sh ./
popd

ls -la $DEPENDENCIES
pushd $ROOT_DIR/$STACK
tar czvf $ARCHIVE_NAME dependencies
popd

find $DD_AGENT_TESTING_DIR -type d -user root -exec chown -R $USER_ID:$USER_ID {} \;
find $DD_AGENT_TESTING_DIR -type f -user root -exec chown $USER_ID:$USER_ID {} \;
