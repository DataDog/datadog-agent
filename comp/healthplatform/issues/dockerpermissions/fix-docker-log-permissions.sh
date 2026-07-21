#!/bin/bash
# Fix Docker log file permissions for Datadog Agent
# This grants the dd-agent user read access to Docker log files using ACLs

set -e

DOCKER_DIR="{{.DockerDir}}"

echo "Granting dd-agent read access to Docker logs..."
setfacl -Rm g:dd-agent:rx "$DOCKER_DIR/containers"
setfacl -Rm g:dd-agent:r "$DOCKER_DIR/containers"/*/*.log
setfacl -Rdm g:dd-agent:rx "$DOCKER_DIR/containers"

echo "Restarting Datadog Agent..."
systemctl restart datadog-agent

echo "Done! Check agent status with: datadog-agent status"
