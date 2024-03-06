#!/bin/bash


# Usage:
#
#   ssh-keygen -f ~/.ssh/sidescanner -t ed25519 -C "sidescannerdebug@datadoghq.com"
#   STACK_OWNER=$(whoami) \
#   STACK_API_KEY=<api_key> \
#   STACK_SUBNET_ID=<subnet> \
#   STACK_SECURITY_GROUP=<sg> \
#   aws-vault exec sso-sandbox-account-admin -- ./aws-deploy.sh
#
#     export DD_SIDESCANNER_IP="XXX.XXX.XXX.XXX"
#     ssh -i ubuntu@$DD_SIDESCANNER_IP
#

set -e

if [ -z "$STACK_API_KEY" ]; then
  >&2 printf "Please provide a \$STACK_API_KEY\n"
  exit 1
fi

if [ -z "$STACK_OWNER" ]; then
  >&2 printf "Please provide a \$STACK_OWNER\n"
  exit 1
fi

if [ -z "${STACK_SUBNET_ID}" ]; then
  >&2 printf "Please provide a \$STACK_SUBNET_ID\n"
  exit 1
fi

if [ -z "${STACK_SECURITY_GROUP}" ]; then
  >&2 printf "Please provide a \$STACK_SECURITY_GROUP\n"
  exit 1
fi

BASEDIR=$(dirname "$0")

# Stack meta
STACK_NAME="DatadogAgentlessScanner-${STACK_OWNER}"
STACK_AWS_REGION="us-east-1"
STACK_PUBLIC_KEY=$(cat "$HOME/.ssh/sidescanner.pub")
STACK_TEMPLATE=$(cat "$BASEDIR/aws-cf.yaml")


printf "validating template %s..." "${STACK_NAME}"
aws cloudformation validate-template \
  --template-body "$STACK_TEMPLATE"
printf "\n"

printf "deleting stack %s..." "$STACK_NAME"
aws cloudformation delete-stack --stack-name "$STACK_NAME" --region "$STACK_AWS_REGION"
aws cloudformation wait stack-delete-complete --stack-name "$STACK_NAME" --region "$STACK_AWS_REGION"
printf "\n"

printf "creating stack %s..." "${STACK_NAME}"
STACK_ARN=$(aws cloudformation create-stack \
  --stack-name "$STACK_NAME" \
  --region "$STACK_AWS_REGION" \
  --template-body "$STACK_TEMPLATE" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameters \
    "ParameterKey=DatadogAPIKey,ParameterValue=${STACK_API_KEY}" \
    "ParameterKey=DatadogScannerOwner,ParameterValue=${STACK_OWNER}" \
    "ParameterKey=DatadogPublicKeyMaterial,ParameterValue=${STACK_PUBLIC_KEY}" \
    "ParameterKey=DatadogSubnetId,ParameterValue=${STACK_SUBNET_ID}" \
    "ParameterKey=DatadogSecurityGroup,ParameterValue=${STACK_SECURITY_GROUP}" \
  --query 'StackId' \
  --output text)


printf " waiting for stack \"%s\" to complete...\n" "${STACK_ARN}"
printf "> https://%s.console.aws.amazon.com/cloudformation/home?region=%s#/stacks/stackinfo?stackId=%s\n" "${STACK_AWS_REGION}" "${STACK_AWS_REGION}" "${STACK_ARN}"

if aws cloudformation wait stack-create-complete --stack-name "$STACK_NAME" --region "$STACK_AWS_REGION" > /dev/null;
then
  printf "\n"
  STACK_INSTANCE_ID=$(aws cloudformation describe-stack-resource \
    --stack-name "$STACK_NAME" \
    --logical-resource-id "DatadogAgentlessScannerInstance" \
    --query 'StackResourceDetail.PhysicalResourceId' \
    --output text)

  STACK_INSTANCE_IP=$(aws ec2 describe-instances \
    --instance-ids "$STACK_INSTANCE_ID" \
    --query 'Reservations[0].Instances[0].PrivateIpAddress' \
    --output text)

  echo "stack creation successful | launched instance: ${INSTANCE_ID}."
  echo "export STACK_INSTANCE_IP=\"${STACK_INSTANCE_IP}\""
  echo "ssh -i ~/.ssh/sidescanner ubuntu@\$STACK_INSTANCE_IP"
else
  printf "\n"
  echo "Stack creation failed. Check the AWS CloudFormation console for details."
fi
