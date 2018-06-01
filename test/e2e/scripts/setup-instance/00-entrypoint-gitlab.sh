#!/usr/bin/env bash

printf '=%.0s' {0..79} ; echo
set -ex

cd "$(dirname $0)"

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

echo "Using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
echo "Running inside a gitlab pipeline, using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"

exec ./02-ec2.sh
