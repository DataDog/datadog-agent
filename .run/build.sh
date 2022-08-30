#! /usr/bin/bash -ex

AGENT_ROOT_DIRECTORY=$(git rev-parse --show-toplevel)

if [[ -z $AGENT_SKIP_VENV ]]; then
  # This script assumes that a python venv is located under the root directory
  AGENT_VENV_DIR=venv
  echo "Using a virtual env located in ${AGENT_VENV_DIR}"
  source "$AGENT_VENV_DIR"/bin/activate
fi

pushd "$AGENT_ROOT_DIRECTORY"

DEFAULT_BUILD_COMMAND="invoke agent.build --build-exclude=systemd"
if [[ -z $BUILD_COMMAND ]]; then
  BUILD_COMMAND=$DEFAULT_BUILD_COMMAND
fi

echo "The following build command was given: ${BUILD_COMMAND}"

DELVE=1 ${BUILD_COMMAND}
popd
