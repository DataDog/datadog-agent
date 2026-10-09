#!/bin/bash
# Grant the dd-agent user access to the Docker socket by adding it to the docker group.

set -e

echo "Adding dd-agent to the docker group..."
usermod -aG docker dd-agent

echo "Restarting Datadog Agent..."
systemctl restart datadog-agent

echo "Done! Check agent status with: datadog-agent status"
