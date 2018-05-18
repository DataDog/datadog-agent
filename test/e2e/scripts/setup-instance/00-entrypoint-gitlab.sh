#!/usr/bin/env bash

set -ex

cd $(dirname $0)

git clean -fdx .

# Generate ssh-key and ignition files
./01-ignition.sh
IGNITION_BASE64=$(cat ignition.json | base64 -w 0)

# TODO remove the IamInstanceProfile
tee specification.json << EOF
{
  "ImageId": "ami-5555ff2a",
  "InstanceType": "t2.medium",
  "Monitoring": {
    "Enabled": false
  },
  "BlockDeviceMappings": [
    {
      "DeviceName": "/dev/xvda",
      "Ebs": {
        "DeleteOnTermination": true,
        "VolumeSize": 15,
        "VolumeType": "gp2"
      }
    }
  ],
  "UserData": "${IGNITION_BASE64}",

  "SubnetId": "subnet-c18341ed",
  "IamInstanceProfile": {
    "Name": "ci-datadog-agent-e2e-runner"
  },
  "SecurityGroupIds": ["sg-0f5617ceb3e5a6c39"]
}
EOF

# This is not elegant...
export DATADOG_AGENT_IMAGE="datadog/agent-dev:${CI_COMMIT_REF_SLUG}"
echo "Using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
echo "Running inside a gitlab pipeline, using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
export DATADOG_AGENT_IMAGE

exec ./02-ec2.sh
