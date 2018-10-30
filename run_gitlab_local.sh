#!/bin/bash

echo "Run a gitlab build step on the local machine"

set +x

gitlab-ci-multi-runner exec docker \
  --cache-type s3 \
  --cache-s3-server-address s3.amazonaws.com \
  --cache-s3-bucket-name ci-runner-cache-eu1 \
  --cache-s3-bucket-location eu-west-1 \
  --cache-s3-access-key $AWS_ACCESS_KEY \
  --cache-s3-secret-key $AWS_SECRET_KEY \
  $@
