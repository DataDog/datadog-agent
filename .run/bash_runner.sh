#! /usr/bin/bash -ex

# This script is a wrapper script for running a bash script either locally or on a remote machine using SSH
# It runs the script in the given path SCRIPT_TO_RUN on a remote machine who's ip is REMOTE_MACHINE_IP, or locally
# if REMOTE_MACHINE_IP is not set

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
  # shellcheck disable=SC2002
  cat "${SCRIPT_TO_RUN}" | ssh -tt "vagrant@$REMOTE_MACHINE_IP"
fi
