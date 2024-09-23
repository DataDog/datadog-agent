#!/usr/bin/bash -ex

# This script logs into the Docker registry, builds a Docker image, and pushes it to Amazon ECR.

# Ensure AWS_ECR_LOGIN_PASSWORD is set
if [[ -z $AWS_ECR_LOGIN_PASSWORD ]]; then
  echo "AWS_ECR_LOGIN_PASSWORD environment variable is not set"
  exit 1
fi

# Ensure IMAGE_NAME and IMAGE_VERSION are set
if [[ -z $IMAGE_NAME || -z $IMAGE_VERSION ]]; then
  echo "IMAGE_NAME and IMAGE_VERSION environment variables must be set"
  exit 1
fi

# Docker registry and image details
AWS_ACCOUNT_ID="601427279990"
REGION="us-east-1"
REPOSITORY_NAME="usm-agent"

# Login to Amazon ECR
echo $AWS_ECR_LOGIN_PASSWORD | docker login --username AWS --password-stdin "$AWS_ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com"

# Build and push the Docker image
inv -e process-agent.build-dev-image \
  --image "$AWS_ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com/$REPOSITORY_NAME/$IMAGE_NAME:$IMAGE_VERSION" \
  --base-image datadog/agent-dev:master-py3 --push
