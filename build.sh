#!/bin/sh -e

ORG_PATH="github.com/DataDog"
REPO_PATH="${ORG_PATH}/datadog-agent"

eval $(go env)

go build -o ./bin/agent ${REPO_PATH}/cmd/agent
