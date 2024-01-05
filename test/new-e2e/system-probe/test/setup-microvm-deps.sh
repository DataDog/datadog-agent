#!/bin/bash

set -eo xtrace

STACK=$1
USER_ID=$2
GROUP_ID=$3
ARCH=$4

DD_AGENT_TESTING_DIR=/datadog-agent
ROOT_DIR=kmt-deps

rm -rf $ROOT_DIR/$STACK/dependencies
mkdir -p $ROOT_DIR/$STACK/dependencies
DEPENDENCIES=$(realpath $ROOT_DIR/$STACK/dependencies)

ARCHIVE_NAME=dependencies-x86_64.tar.gz
CLANG_BPF=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/clang-bpf
LLC_BPF=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/llc-bpf
GO_BIN=go/bin
GOTESTSUM=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/gotestsum
TEST2JSON=$DD_AGENT_TESTING_DIR/test/kitchen/site-cookbooks/dd-system-probe-check/files/default/test2json
EMBEDDED_BIN=opt/datadog-agent/embedded/bin
EMBEDDED_INC=opt/datadog-agent/embedded/include
SYSTEM_PROBE_TESTS=/opt/system-probe-tests

[ -f $TEST2JSON ] || sudo cp $(go env GOTOOLDIR)/test2json $TEST2JSON

pushd $DEPENDENCIES
mkdir -p $EMBEDDED_BIN
sudo install -d -m 0777 -o $USER_ID -g $GROUP_ID $SYSTEM_PROBE_TESTS
cp $CLANG_BPF $EMBEDDED_BIN
cp $LLC_BPF $EMBEDDED_BIN
mkdir -p $EMBEDDED_INC
mkdir -p $GO_BIN
cp $GOTESTSUM $GO_BIN
cp $TEST2JSON $GO_BIN
mkdir junit testjson pkgjson
cp $DD_AGENT_TESTING_DIR/test/new-e2e/system-probe/test/micro-vm-init.sh ./

pushd "${DD_AGENT_TESTING_DIR}/test/new-e2e/system-probe/test-runner" && GOOS=linux go build -o "${DEPENDENCIES}/test-runner" && popd
pushd "${DD_AGENT_TESTING_DIR}/test/new-e2e/system-probe/test-json-review" && GOOS=linux go build -o "${DEPENDENCIES}/test-json-review" && popd

popd

ls -la $DEPENDENCIES
pushd $ROOT_DIR/$STACK
tar czvf $ARCHIVE_NAME dependencies
popd

