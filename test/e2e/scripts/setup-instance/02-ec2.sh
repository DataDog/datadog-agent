#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo

cd "$(dirname "$0")"

COMMIT_ID=$(git rev-parse --verify HEAD)
export COMMIT_ID
BRANCH=$(git rev-parse --abbrev-ref HEAD)
COMMIT_USER=$(git log -1 --pretty=format:'%ae' | tr -d '[:space:]')

# If not using the default value, remember to change the following settings:
# - AMI
# - security groups ids
# - subnet ids
REGION="${REGION:-us-east-1}"

SPOT_REQUEST_ID=$(aws ec2 request-spot-instances \
                      --spot-price "0.340" \
                      --instance-count 1 \
                      --type "one-time" \
                      --valid-until $(($(date +%s) + 3300)) \
                      --launch-specification file://specification.json \
                      --region "${REGION}" \
                      --query "SpotInstanceRequests[*].SpotInstanceRequestId" \
                      --output text)

# Try finding EC2 Spot instance 500 times
n=1
until [[ -n ${INSTANCE_ID:+x} ]] || [ $n -ge 500 ]; do
    sleep 5
    INSTANCE_ID=$(aws ec2 describe-spot-instance-requests \
                      --region "${REGION}" \
                      --spot-instance-request-ids "${SPOT_REQUEST_ID}" \
                      --query "SpotInstanceRequests[*].InstanceId" \
                      --output text)
    n=$(( $n + 1 ))
done

if [[ -z ${INSTANCE_ID:+x} ]]; then
    echo "Failed to allocate end to end test EC2 Spot instance after $n attempts"
    exit 1
fi

aws ec2 create-tags --resources "${SPOT_REQUEST_ID}" "${INSTANCE_ID}" \
    --region "${REGION}" \
    --tags \
    "Key=repository,Value=github.com/DataDog/datadog-agent" \
    "Key=branch,Value='${BRANCH}'" \
    "Key=commit,Value='${COMMIT_ID:0:8}'" \
    "Key=user,Value='${COMMIT_USER}'"

until [[ -n ${INSTANCE_ENDPOINT:+x} ]]; do
    sleep 5
    INSTANCE_ENDPOINT=$(aws ec2 describe-instances \
                            --instance-ids "${INSTANCE_ID}" \
                            --region "${REGION}" \
                            --query "Reservations[*].Instances[*].PrivateIpAddress" \
                            --output text)
done

aws ec2 cancel-spot-instance-requests \
    --spot-instance-request-ids "${SPOT_REQUEST_ID}" \
    --region "${REGION}"

exec ./03-ssh.sh "${INSTANCE_ENDPOINT}"
