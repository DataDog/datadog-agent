#! /usr/bin/bash -ex

AGENT_ROOT_DIRECTORY=$(git rev-parse --show-toplevel)

# This script assumes that a python venv is located under the root directory
DD_VENV_DIR=venv
source "$DD_VENV_DIR"/bin/activate

pushd "$AGENT_ROOT_DIRECTORY"
DELVE=1 invoke agent.build --build-exclude=systemd
popd
