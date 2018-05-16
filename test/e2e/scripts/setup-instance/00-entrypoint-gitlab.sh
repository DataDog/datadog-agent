#!/usr/bin/env bash

set -ex

cd $(dirname $0)

git clean -fdx .

# Generate ssh-key and ignition files
./01-ignition.sh
IGNITION_BASE64=$(cat ignition.json | base64 -w 0)

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
export DATADOG_AGENT_IMAGE="486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent/agent:v${CI_PIPELINE_ID}-${CI_COMMIT_SHA:0:7}${TAG_SUFFIX}"
echo "Using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
echo "Running inside a gitlab pipeline, using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
export DATADOG_AGENT_IMAGE

aws ecr get-login --region us-east-1 --no-include-email --registry-ids 486234852809 > ecr.login

set +x
cat << EOF > kube-script.sh
#!/bin/bash -e

kubectl create secret docker-registry ecr-credentials \
  --docker-server="https://486234852809.dkr.ecr.us-east-1.amazonaws.com" \
  --docker-username="AWS" \
  --docker-password="$(cat ecr.login | cut -f6 -d ' ')" \
  --docker-email=dev@null.com

kubectl patch serviceaccount default -p '{"imagePullSecrets": [{"name": "ecr"}]}'
EOF
chmod +x kube-script.sh

exec ./02-ec2.sh
