#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo

BASE64_FLAGS="-w 0"
# OSX with 2 types of base64 binary in PATH ...
if [[ $(uname) == "Darwin" ]]
then
    echo "Currently running over Darwin"
    # shellcheck disable=SC2086
    echo "osx base64" | base64 ${BASE64_FLAGS} || {
        echo "current base64 binary does not support ${BASE64_FLAGS}"
        BASE64_FLAGS=""
    }
fi

set -e

cd "$(dirname "$0")"

git clean -fdx .

# Generate ssh-key and ignition files
./01-ignition.sh
# shellcheck disable=SC2086
IGNITION_BASE64=$(base64 ${BASE64_FLAGS} ignition.json)

REGION="${REGION:-us-east-1}"
UPDATE_STREAM="${UPDATE_STREAM:-stable}"
AMI="$(curl "https://builds.coreos.fedoraproject.org/streams/${UPDATE_STREAM}.json" | jq -r ".architectures.x86_64.images.aws.regions.\"$REGION\".image")"

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

  "SubnetId": "subnet-b89e00e2",
  "SecurityGroupIds": ["sg-7fedd80a"]
}
EOF

export CI_COMMIT_SHORT_SHA=${CI_COMMIT_SHORT_SHA:-$(git describe --tags --always --dirty --match 7.\*)}

exec ./02-ec2.sh
