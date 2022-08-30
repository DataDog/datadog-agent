#! /usr/bin/bash -ex

AGENT_ROOT_DIRECTORY=$(git rev-parse --show-toplevel)

pushd "$AGENT_ROOT_DIRECTORY"
DEFAULT_BINARY_TO_RUN="./bin/agent/agent"
if [[ -z $BINARY_TO_RUN ]]; then
  BINARY_TO_RUN=$DEFAULT_BINARY_TO_RUN
fi

echo "Running the following binary using dlv: ${BINARY_TO_RUN}"
sudo dlv --listen=0.0.0.0:2345 --headless=true --api-version=2 --check-go-version=false --only-same-user=false exec ${BINARY_TO_RUN} -- run -c ./bin/agent/dist/datadog.yaml
popd
