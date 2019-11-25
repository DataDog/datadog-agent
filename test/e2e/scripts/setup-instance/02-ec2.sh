#!/usr/bin/env bash

printf '=%.0s' {0..79} ; echo
set -ex
set -o pipefail

cd "$(dirname $0)"

export COMMIT_ID=$(git rev-parse --verify HEAD)
BRANCH=$(git rev-parse --abbrev-ref HEAD)
COMMIT_USER=$(git log -1 --pretty=format:'%ae' | tr -d [:space:])

# If not using the default value, remember to change the following settings:
# - AMI
# - security groups ids
# - subnet ids
REGION="${REGION:-us-east-1}"

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
INSTANCE_ENDPOINT=""
while true
do
    aws ec2 describe-instances \
        --instance-ids ${INSTANCE_ID} \
        --region ${REGION} | tee instance-id.json

    INSTANCE_ENDPOINT=$(jq -re .Reservations[].Instances[].PrivateIpAddress instance-id.json)
    if [[ $? != "0" ]]
    then
        sleep 5
        continue
    fi
    break
done

aws ec2 cancel-spot-instance-requests \
    --spot-instance-request-ids ${SPOT_REQUEST_ID} \
    --region ${REGION}

exec ./03-ssh.sh ${INSTANCE_ENDPOINT}
