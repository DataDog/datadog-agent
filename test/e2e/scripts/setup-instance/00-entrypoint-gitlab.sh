#!/usr/bin/env bash

set -euo pipefail

printf '=%.0s' {0..79} ; echo

cd "$(dirname "$0")"

git clean -fdx .

# Generate ssh-key and ignition files
./01-ignition.sh
IGNITION_BASE64=$(base64 -w 0 ignition.json)

REGION="${REGION:-us-east-1}"
UPDATE_STREAM="${UPDATE_STREAM:-stable}"
if [ -z "${AMI+x}" ]; then
  AMI="$(curl "https://builds.coreos.fedoraproject.org/streams/${UPDATE_STREAM}.json" | jq -r ".architectures.x86_64.images.aws.regions.\"$REGION\".image")"
fi
ARGO_WORKFLOW=${ARGO_WORKFLOW:-''}

# TODO remove the IamInstanceProfile
tee specification.json << EOF
{
  "ImageId": "${AMI}",
  "InstanceType": "t3.2xlarge",
  "Monitoring": {
    "Enabled": false
  },
  "BlockDeviceMappings": [
    {
      "DeviceName": "/dev/xvda",
      "Ebs": {
        "DeleteOnTermination": true,
        "VolumeSize": 50,
        "VolumeType": "gp2"
      }
    }
  ],
  "UserData": "${IGNITION_BASE64}",

  "SubnetId": "subnet-05d7c6b1b5cfea811",
  "IamInstanceProfile": {
    "Name": "ci-datadog-agent-e2e-runner"
  },
  "SecurityGroupIds": ["sg-019917348cb0eb7e7"]
}
EOF

echo "Running inside a gitlab pipeline,"
echo "using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
echo "using DATADOG_CLUSTER_AGENT_IMAGE=${DATADOG_CLUSTER_AGENT_IMAGE}"
echo "using ARGO_WORKFLOW=${ARGO_WORKFLOW}"

# Check if the image is hosted on a docker registry and if it's available
echo "${DATADOG_AGENT_IMAGE} is hosted on a docker registry, checking if it's available"
IMAGE_REPOSITORY=${DATADOG_AGENT_IMAGE%:*}
IMAGE_TAG=${DATADOG_AGENT_IMAGE#*:}
if ! curl -Lfs --head "https://hub.docker.com/v2/repositories/${IMAGE_REPOSITORY}/tags/${IMAGE_TAG}" > /dev/null ; then
        echo "The DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE} is not available on DockerHub"
        echo "Ensure that the manual jobs in dev_container_deploy has been run/rerun"
        echo "*dev_branch* -> k8s-e2e-*-dev"
        echo "*dev_master* -> k8s-e2e-*-main"
        exit 2
fi

echo "${DATADOG_CLUSTER_AGENT_IMAGE} is hosted on a docker registry, checking if it's available"
IMAGE_REPOSITORY=${DATADOG_CLUSTER_AGENT_IMAGE%:*}
IMAGE_TAG=${DATADOG_CLUSTER_AGENT_IMAGE#*:}
if ! curl -Lfs --head "https://hub.docker.com/v2/repositories/${IMAGE_REPOSITORY}/tags/${IMAGE_TAG}" > /dev/null ; then
        echo "The DATADOG_CLUSTER_AGENT_IMAGE=${DATADOG_CLUSTER_AGENT_IMAGE} is not available on DockerHub"
        echo "Ensure that the manual jobs in dev_container_deploy has been run/rerun"
        echo "*dev_branch* -> k8s-e2e-*-dev"
        echo "*dev_master* -> k8s-e2e-*-main"
        exit 2
fi

exec ./02-ec2.sh
