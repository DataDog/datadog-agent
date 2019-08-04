#!/bin/sh

if [[ -z "$STS_API_KEY" ]]; then
    echo "You must set an STS_API_KEY environment variable to run the StackState Trace Agent container"
    exit 1
fi

/opt/stackstate-agent/bin/agent/trace-agent -config /etc/stackstate-agent/stackstate-docker.yaml
