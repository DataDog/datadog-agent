#! /usr/bin/bash -ex
set -e

# This script is a wrapper script for running a bash script either locally or on a remote machine using SSH
# It runs the script in the given path SCRIPT_TO_RUN on a remote machine who's ip is REMOTE_MACHINE_IP, or locally
# if REMOTE_MACHINE_IP is not set
# Note that this script assumes that the remote machine was configured with the agent dev box vagrant scripts, for more
# information about this setup see https://github.com/DataDog/croissant-integration-resources/tree/master/agent-dev_box

# Add all configuration environment variables to the current context
source .run/configuration.sh

if [[ -z $SCRIPT_TO_RUN ]]; then
  echo "SCRIPT_TO_RUN environment variable must be set"
  exit
fi

if [[ -z $REMOTE_MACHINE_IP ]]; then
  echo "REMOTE_MACHINE_IP environment variable was not set, assuming local configuration"
  source "${SCRIPT_TO_RUN}"
  exit
else
  echo "REMOTE_MACHINE_IP is set to $REMOTE_MACHINE_IP, using remote configuration"
  DD_AGENT_ROOT_DIR="/git/datadog-agent"

  # Exporting all relevant environment variables from current session os it will be available for the script in the SSH session
  # Then change directory to the agent root directory to preserve the behaviour of a local run

  # shellcheck disable=SC2002
  cat "${SCRIPT_TO_RUN}" | ssh -tt "vagrant@$REMOTE_MACHINE_IP" \
   "export AGENT_SKIP_VENV=${DD_AGENT_ROOT_DIR} BUILD_COMMAND='${BUILD_COMMAND}' BINARY_TO_RUN='${BINARY_TO_RUN}' AGENT_VENV_DIR='${AGENT_VENV_DIR}' DLV_PORT_TO_BIND='${DLV_PORT_TO_BIND}' BINARY_ARGUMENTS='${BINARY_ARGUMENTS}';cd ${DD_AGENT_ROOT_DIR};bash --login"
fi
