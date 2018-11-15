#!/bin/bash -x

echo "Run a gitlab build step on the local machine"

#  --env SIGNING_KEY_ID="$SIGNING_KEY_ID" \
#  --env SIGNING_PRIVATE_KEY="$SIGNING_PRIVATE_KEY" \
#  --env SIGNING_PUBLIC_KEY="$SIGNING_PUBLIC_KEY"

gitlab-ci-multi-runner exec docker \
  --cache-type s3 \
  --cache-s3-server-address s3.amazonaws.com \
  --cache-s3-bucket-name ci-runner-cache-eu1 \
  --cache-s3-bucket-location eu-west-1 \
  --cache-s3-access-key $AWS_ACCESS_KEY \
  --cache-s3-secret-key $AWS_SECRET_KEY \
  --env SIGNING_KEY_ID="$SIGNING_KEY_ID" \
  --env SIGNING_PRIVATE_KEY="$SIGNING_PRIVATE_KEY" \
  --env SIGNING_PUBLIC_KEY="$SIGNING_PUBLIC_KEY" \
  --env AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  --env AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  --env AWS_REGION="$AWS_REGION" \
  --env AWS_DEFAULT_REGION="$AWS_DEFAULT_REGION" \
  $@
