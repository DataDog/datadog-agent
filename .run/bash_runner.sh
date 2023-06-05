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

  # We don't want to override existing environment variables in the remote machine
  # So we get all remote environment variables names, and "subtract" them from the local environment variables

  # Getting all environment variables names in the remote machine
  remote_env=$(ssh -tt "vagrant@$REMOTE_MACHINE_IP" env | cut -d "=" -f1)

  # We will use grep with the -v flag (inverse mode) to exclude the remote environment variables from the local ones
  # To do this, we need to transform the remote environment variables into the patterns format which grep expect (`grep -e FIRST_ENV -e SECOND_ENV ...`)
  remote_env_array=("$remote_env")
  remote_env_array_as_grep_patterns=()
  for env in "${remote_env_array[@]}"; do remote_env_array_as_grep_patterns+=(-e "$env"); done

  # Ignore local environment variables that could cause problem in the remote machine
  ENV_IGNORE_LIST=("TMPDIR" "GOPRIVATE" "GOROOT" "GOPATH")
  for env in "${ENV_IGNORE_LIST[@]}"; do remote_env_array_as_grep_patterns+=(-e "$env"); done

  # Get all environment variables enclosed in quote so we'll handle special characters correctly ($, =, etc..)
  env_vars=""
  for var in $(env | cut -d= -f1); do
      env_vars+="$(echo "$var"=\"${!var}\")"$'\n'
  done

  # Finally create the environment variable to inject list in the format that works with sh `ssh` command
  env_variables_to_inject=$(echo "$env_vars" | grep -v -w "${remote_env_array_as_grep_patterns[@]}" | tr '\n' ' ')
  # shellcheck disable=SC2002
  cat "${SCRIPT_TO_RUN}" | ssh -tt "vagrant@$REMOTE_MACHINE_IP" \
  "export $env_variables_to_inject;cd ${DD_AGENT_ROOT_DIR};bash --login"
fi
