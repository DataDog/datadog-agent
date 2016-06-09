#!/bin/sh -e

ORG_PATH="github.com/DataDog"
REPO_PATH="${ORG_PATH}/datadog-agent"
BIN_PATH="./bin/agent"

eval $(go env)

go build -o ${BIN_PATH}/agent ${REPO_PATH}/cmd/agent
cp -r ./pkg/py/dist/ ${BIN_PATH}/dist/
