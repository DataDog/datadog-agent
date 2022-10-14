#! /usr/bin/bash -ex
set -e

# This script is responsible for running a given module using dlv, so it will be possible to debug the module
# It can get the following parameters:
# BINARY_TO_RUN - The name of the binary to run
# BINARY_ARGUMENTS - The argument string for the specific binary
# DLV_PORT_TO_BIND - The port to be used by dlv. This allows to debug several components simultaneously on the same machine

AGENT_ROOT_DIRECTORY=$(git rev-parse --show-toplevel)

pushd "${AGENT_ROOT_DIRECTORY}"
BINARY_TO_RUN=${BINARY_TO_RUN:-"./bin/agent/agent"}

echo "Running the following command using dlv: ${BINARY_TO_RUN} ${BINARY_ARGUMENTS}"
DLV_PORT_TO_BIND=${DLV_PORT_TO_BIND:-2345}
BINARY_ARGUMENTS=${BINARY_ARGUMENTS:-"run -c ./bin/agent/dist/datadog.yaml"}
DLV_BINARY_PATH=$(which dlv)
# Removing quotes from the binary arguments.
BINARY_ARGUMENTS="${BINARY_ARGUMENTS%\"}"
BINARY_ARGUMENTS="${BINARY_ARGUMENTS#\"}"
sudo -E ${DLV_BINARY_PATH} --listen=0.0.0.0:${DLV_PORT_TO_BIND} --headless=true --api-version=2 --check-go-version=false --only-same-user=false exec \
 ${BINARY_TO_RUN} -- ${BINARY_ARGUMENTS}
popd
