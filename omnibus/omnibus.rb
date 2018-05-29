# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

windows_arch :x86_64
# Don't append a timestamp to the package version
append_timestamp false

# Do not set this environment variable if building locally.
# This cache is only necessary because Datadog is building
# the agent over and over again in a highly distributed environment.
if ENV["S3_OMNIBUS_CACHE_BUCKET"]
  use_s3_caching true
  s3_bucket ENV["S3_OMNIBUS_CACHE_BUCKET"]
  s3_endpoint "https://s3.amazonaws.com"
  s3_region 'us-east-1'
  s3_force_path_style true
  s3_authenticated_download true
  if ENV['WINDOWS_BUILDER']
    s3_role true
    s3_role_arn 'arn:aws:iam::486234852809:role/ci-datadog-agent'
    s3_role_session_name 'datadog-agent-builder'
    s3_sts_creds_instance_profile true
  else
    s3_instance_profile true
  end
end
