# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "./lib/project_extension.rb"

if ENV["WINDOWS_BUILD_32_BIT"]
    windows_arch :x86
else
    windows_arch :x86_64
end
# Don't append a timestamp to the package version
append_timestamp false


if ENV["OMNIBUS_WORKERS_OVERRIDE"]
  workers ENV["OMNIBUS_WORKERS_OVERRIDE"].to_i
end

# Do not set this environment variable if building locally.
# This cache is only necessary because Datadog is building
# the agent over and over again in a highly distributed environment.
if ENV["S3_OMNIBUS_CACHE_BUCKET"]
  use_s3_caching true
  s3_bucket ENV["S3_OMNIBUS_CACHE_BUCKET"]
  s3_endpoint "https://s3.amazonaws.com"
  s3_region 'us-east-1'
  s3_force_path_style true
  s3_authenticated_download ENV.fetch('S3_OMNIBUS_CACHE_ANONYMOUS_ACCESS', '') == '' ? true : false
  if ENV['WINDOWS_BUILDER']
    s3_profile "default"
    # Get the credentials path and expand Windows environment variables
    default_path = File.join(ENV['USERPROFILE'] || '', '.aws', 'credentials')
    credentials_path = ENV.fetch('AWS_SHARED_CREDENTIALS_FILE', default_path)
    s3_credentials_file_path credentials_path

    # Check and log if credentials file exists
    if File.exist?(credentials_path)
      puts "AWS credentials file found at: #{credentials_path}"
    else
      puts "WARNING: AWS credentials file not found at: #{credentials_path}"
      puts "This may cause S3 caching authentication issues."
    end
  else
    s3_instance_profile true
  end
end

# This setting can be overriden per-project (which is the case for the agent builds)
use_git_caching false

