#!/bin/bash
# Fix AWS IMDSv2 hop limit for containerized Datadog Agent
# Run this script on the EC2 host (not inside the container).
# Increases the IMDSv2 hop limit from 1 to 2 so that containers can reach
# the instance metadata service at 169.254.169.254.

set -e

echo "Fetching IMDSv2 token..."
if ! TOKEN=$(curl -sf -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600" --max-time 5) || [ -z "$TOKEN" ]; then
    echo "ERROR: Could not fetch IMDSv2 token. Run this script on the EC2 host, not inside a container." >&2
    exit 1
fi

echo "Fetching EC2 instance ID..."
if ! INSTANCE_ID=$(curl -sf "http://169.254.169.254/latest/meta-data/instance-id" -H "X-aws-ec2-metadata-token: $TOKEN" --max-time 5) || [ -z "$INSTANCE_ID" ]; then
    echo "ERROR: Could not fetch EC2 instance ID using the IMDSv2 token." >&2
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
