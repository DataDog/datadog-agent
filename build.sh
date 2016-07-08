#!/bin/sh -e

ORG_PATH="github.com/DataDog"
REPO_PATH="${ORG_PATH}/datadog-agent"
BIN_PATH="./bin/agent"

eval $(go env)
export GO15VENDOREXPERIMENT="1"

go build -o ${BIN_PATH}/agent ${REPO_PATH}/cmd/agent
cp -r ./pkg/collector/check/py/dist/ ${BIN_PATH}/dist/
