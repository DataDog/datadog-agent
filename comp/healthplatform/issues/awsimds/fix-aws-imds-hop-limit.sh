#!/bin/bash
# Fix AWS IMDSv2 hop limit for containerized Datadog Agent
# Run this script on the EC2 host (not inside the container).
# Increases the IMDSv2 hop limit from 1 to 2 so that containers can reach
# the instance metadata service at 169.254.169.254.

set -e

echo "Fetching EC2 instance ID..."
INSTANCE_ID=$(curl -s --max-time 5 http://169.254.169.254/latest/meta-data/instance-id 2>/dev/null || true)

if [ -z "$INSTANCE_ID" ]; then
    echo "ERROR: Could not fetch EC2 instance ID. Ensure this script is run on the EC2 host, not inside a container."
    exit 1
fi

echo "Instance ID: $INSTANCE_ID"
echo "Updating IMDSv2 hop limit to 2..."
aws ec2 modify-instance-metadata-options \
    --instance-id "$INSTANCE_ID" \
    --http-put-response-hop-limit 2 \
    --http-endpoint enabled

echo "Done! IMDSv2 hop limit set to 2."
echo "Restart the Datadog Agent container to pick up the correct EC2 hostname."
