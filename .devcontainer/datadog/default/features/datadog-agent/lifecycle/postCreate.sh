#!/bin/bash


# Run install tools
cd ~/dd/datadog-agent

# Tweaking environment variables, will be removed once the Docker image is updated.
# Make sure we can dynamically install dda dependencies
export DDA_NO_DYNAMIC_DEPS=0
# Unset GOPATH otherwise it is set to /go in the build image base
unset GOPATH

dda inv install-tools 2>&1 | tee "/home/bits/.install-tools.log"
dda inv vscode.setup 2>&1 | tee "/home/bits/.vscode-setup.log"
