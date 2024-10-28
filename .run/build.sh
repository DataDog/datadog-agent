#! /usr/bin/bash -ex
set -e

# This script is responsible for building a given module in the agent
# It can get the following parameters:
# AGENT_SKIP_VENV - Set this to true if you want to use the default python instead of a virtual env
# AGENT_VENV_DIR - Set this to the path of the virtual environment you wish to use. By default it assumes that it's located in the root directory
#                  under a directory called venv
# BUILD_COMMAND - the build command to run, depending on the specific module in the agent

AGENT_ROOT_DIRECTORY=$(git rev-parse --show-toplevel)

if [[ -z $AGENT_SKIP_VENV ]]; then
  AGENT_VENV_DIR=${AGENT_VENV_DIR:-"venv"}
  echo "Using a virtual env located in ${AGENT_VENV_DIR}"
  source "$AGENT_VENV_DIR"/bin/activate
fi

pushd "${AGENT_ROOT_DIRECTORY}"

BUILD_COMMAND=${BUILD_COMMAND:-"invoke agent.build --build-exclude=systemd"}
echo "The following build command was given: ${BUILD_COMMAND}"

DELVE=1 eval "${BUILD_COMMAND}"
popd
