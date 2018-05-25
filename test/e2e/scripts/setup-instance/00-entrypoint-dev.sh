#!/usr/bin/env bash

printf '=%.0s' {0..79} ; echo
set -ex

cd "$(dirname $0)"

git clean -fdx .

# Generate ssh-key and ignition files
./01-ignition.sh

if [[ $(uname) == "Linux" ]]
then
    BASE64_FLAGS="-w 0"
fi

IGNITION_BASE64=$(cat ignition.json | base64 ${BASE64_FLAGS})

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

  "SubnetId": "subnet-b89e00e2",
  "SecurityGroupIds": ["sg-7fedd80a"]
}
EOF

exec ./02-ec2.sh
