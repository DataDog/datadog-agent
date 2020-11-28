#!/bin/bash

docker volume create dd-agent-omnibus
docker volume create dd-agent-gems

docker rm -f code
docker create -v /go/src/github.com/DataDog/datadog-agent --name code busybox

set -e -o pipefail
tar cf - --exclude-vcs-ignores . | docker cp - code:/go/src/github.com/DataDog/datadog-agent

docker run \
    --volumes-from code \
    -v "dd-agent-omnibus:/omnibus" \
    -v "dd-agent-gems:/gems" \
    --workdir /go/src/github.com/DataDog/datadog-agent \
    datadog/agent-buildimages-rpm_x64 inv -e agent.omnibus-build --base-dir=/omnibus --gem-path=/gems

docker cp code:/go/src/github.com/DataDog/datadog-agent/omnibus/pkg ./omnibus/pkg
echo "Build output is in: $(pwd)/omnibus/pkg/"
ls -lh "$(pwd)/omnibus/pkg/"
