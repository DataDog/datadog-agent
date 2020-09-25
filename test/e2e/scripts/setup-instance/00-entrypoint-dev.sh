#!/usr/bin/env bash

printf '=%.0s' {0..79} ; echo
set -x

BASE64_FLAGS="-w 0"
# OSX with 2 types of base64 binary in PATH ...
if [[ $(uname) == "Darwin" ]]
then
    echo "Currently running over Darwin"
    echo "osx base64" | base64 ${BASE64_FLAGS} || {
        echo "current base64 binary does not support ${BASE64_FLAGS}"
        BASE64_FLAGS=""
    }
fi

set -e

cd "$(dirname $0)"

git clean -fdx .

# Generate ssh-key and ignition files
./01-ignition.sh
IGNITION_BASE64=$(base64 ${BASE64_FLAGS} ignition.json)

REGION="${REGION:-us-east-1}"
UPDATE_STREAM="${UPDATE_STREAM:-stable}"
AMI="$(curl "https://builds.coreos.fedoraproject.org/streams/${UPDATE_STREAM}.json" | jq -r ".architectures.x86_64.images.aws.regions.\"$REGION\".image")"

tee specification.json << EOF
{
  "ImageId": "${AMI}",
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

  "SubnetId": "subnet-b89e00e2",
  "SecurityGroupIds": ["sg-7fedd80a"]
}
EOF

exec ./02-ec2.sh
