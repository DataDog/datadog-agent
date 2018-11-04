#!/bin/bash

# Set a fallback empty config with no AD
# Don't override /etc/stackstate-agent/stackstate.yaml if it exists
if [[ ! -e /etc/stackstate-agent/stackstate.yaml ]]; then
    touch  /etc/stackstate-agent/stackstate.yaml
fi
