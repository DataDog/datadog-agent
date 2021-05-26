#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

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
                      --spot-price "0.150" \
                      --instance-count 1 \
                      --type "one-time" \
                      --valid-until $(($(date +%s) + 3300)) \
                      --launch-specification file://specification.json \
                      --region "${REGION}" \
                      --query "SpotInstanceRequests[*].SpotInstanceRequestId" \
                      --output text)

until [[ -n ${INSTANCE_ID:+x} ]]; do
    sleep 5
    INSTANCE_ID=$(aws ec2 describe-spot-instance-requests \
                      --region "${REGION}" \
                      --spot-instance-request-ids "${SPOT_REQUEST_ID}" \
                      --query "SpotInstanceRequests[*].InstanceId" \
                      --output text)
done

aws ec2 create-tags --resources "${SPOT_REQUEST_ID}" "${INSTANCE_ID}" \
    --region "${REGION}" \
    --tags \
    Key=repository,Value=github.com/DataDog/datadog-agent \
    Key=branch,Value="${BRANCH}" \
    Key=commit,Value="${COMMIT_ID:0:8}" \
    Key=user,Value="${COMMIT_USER}"

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
