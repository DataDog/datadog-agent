#!/usr/bin/env bash

set -ex

cd $(dirname $0)

# Generate ssh-key and ignition files
./01-ignition.sh
IGNITION_BASE64=$(cat ignition.json | base64 -w 0)

tee specification.json << EOF
{
  "ImageId": "ami-9e2685e3",
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
