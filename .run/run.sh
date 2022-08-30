#! /usr/bin/bash -ex

AGENT_ROOT_DIRECTORY=$(git rev-parse --show-toplevel)

pushd "$AGENT_ROOT_DIRECTORY"
echo "Running the agent using dlv"
sudo dlv --listen=0.0.0.0:2345 --headless=true --api-version=2 --check-go-version=false --only-same-user=false exec ./bin/agent/agent -- run -c ./bin/agent/dist/datadog.yaml
popd
