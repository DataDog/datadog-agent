#!/usr/bin/env bash

set -ex
set -o pipefail

cd $(dirname $0)

export COMMIT_ID=$(git rev-parse --verify HEAD)
BRANCH=$(git rev-parse --abbrev-ref HEAD)
COMMIT_USER=$(git log -1 --pretty=format:'%an')

REGION="us-east-2"

# Generate ssh-key and ignition files
./01-ignition.sh
IGNITION_BASE64=$(cat ignition.json | base64 -w 0)

cat << EOF > specification.json
{
  "ImageId": "ami-5d6e5e38",
  "InstanceType": "t2.medium",
  "Monitoring": {
    "Enabled": false
  },
  "IamInstanceProfile": {
    "Name": "ci-datadog-agent-e2e-runner"
  },
  "SecurityGroupIds": ["sg-0f5617ceb3e5a6c39"],
  "BlockDeviceMappings": [
    {
      "DeviceName": "/dev/xvda",
      "Ebs": {
        "DeleteOnTermination": true,
        "SnapshotId": "snap-0647640d5fbac6d62",
        "VolumeSize": 15,
        "VolumeType": "gp2"
      }
    }
  ],
  "UserData": "${IGNITION_BASE64}"
}
EOF

jq . specification.json

aws ec2 request-spot-instances \
    --spot-price "0.015" \
    --instance-count 1 \
    --type "one-time" \
    --valid-until $(($(date +%s) + 3300)) \
    --launch-specification file://specification.json \
    --region ${REGION} | tee spot-instance-request.json

SPOT_REQUEST_ID=$(jq -re .SpotInstanceRequests[].SpotInstanceRequestId spot-instance-request.json)


while true
do
    aws ec2 describe-spot-instance-requests \
        --region ${REGION} \
        --spot-instance-request-ids ${SPOT_REQUEST_ID} | tee spot-request-id.json

    jq -re .SpotInstanceRequests[].InstanceId spot-request-id.json  || {
        sleep 5
        continue
    }
    break
done

INSTANCE_ID=$(jq -re .SpotInstanceRequests[].InstanceId spot-request-id.json)

aws ec2 create-tags --resources ${SPOT_REQUEST_ID} ${INSTANCE_ID} \
    --region ${REGION} \
    --tags \
    Key=repository,Value=github.com/DataDog/datadog-agent \
    Key=branch,Value=${BRANCH} \
    Key=commit,Value=${COMMIT_ID:0:8} \
    Key=user,Value=${COMMIT_USER}

set +e
PUBLIC_DNS_NAME=""
while true
do
    aws ec2 describe-instances \
        --instance-ids ${INSTANCE_ID} \
        --region ${REGION} | tee instance-id.json

    PUBLIC_DNS_NAME=$(jq -re .Reservations[].Instances[].PublicDnsName instance-id.json)
    if [[ "${PUBLIC_DNS_NAME}x" == "x" ]]
    then
        sleep 5
        continue
    fi
    break
done

until nc -w 1 -zvv ${PUBLIC_DNS_NAME} 22
do
    sleep 5
done

set -e
exec ./02-ssh.sh ${PUBLIC_DNS_NAME}
