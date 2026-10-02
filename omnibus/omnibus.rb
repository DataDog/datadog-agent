# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "./lib/project_extension.rb"
require 'fileutils'
require 'tmpdir'

repo_root = File.expand_path(File.join(__dir__, ".."))
shim_dir  = File.join(Dir.tmpdir, "omnibazelisk")
FileUtils.mkdir_p(shim_dir)
if Gem.win_platform?
  bazel_exe = `where bazelisk.exe 2>NUL`.lines.first&.chomp
  raise "bazelisk.exe not found in PATH" if bazel_exe.nil?
  %w[bazel.bat bazelisk.bat].each do |name|
    File.write(File.join(shim_dir, name), <<~BAT)
      @echo off
      cd /d "#{repo_root}" || exit /b 2
      "#{bazel_exe}" %*
      exit /b %errorlevel%
    BAT
  end
else
  bazel = `which bazelisk 2>/dev/null`.chomp
  raise "bazelisk not found in PATH" if bazel.empty?
  %w[bazel bazelisk].each do |name|
    shim = File.join(shim_dir, name)
    File.write(shim, <<~SH)
      #!/usr/bin/env bash
      set -eu
      cd "#{repo_root}"
      marker="#{shim_dir}/last_call.marker"
      if [ "${1:-}" = "run" ] || [ "${1:-}" = "build" ]; then
        echo "skyframe summary before: $*" >&2
        "#{bazel}" dump --skyframe=summary >&2 || true
        "#{bazel}" info used-heap-size-after-gc max-heap-size gc-count gc-time >&2 || true
        if [ -f "$marker" ]; then
          echo "files modified in repo since previous bazel call finished:" >&2
          find . -path ./.cache -prune -o -path ./.git -prune -o -type f -newer "$marker" -print 2>/dev/null | head -50 >&2 || true
          echo "end of modified files" >&2
        fi
        rc=0
        "#{bazel}" "$1" --announce_rc "${@:2}" || rc=$?
        echo "skyframe summary after: $*" >&2
        "#{bazel}" dump --skyframe=summary >&2 || true
        touch "$marker"
        exit $rc
      fi
      exec "#{bazel}" "$@"
    SH
    File.chmod(0755, shim)
  end
end
ENV['PATH'] = "#{shim_dir}#{File::PATH_SEPARATOR}#{ENV['PATH']}"

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
  if ENV['WINDOWS_BUILDER'] || RbConfig::CONFIG['host_os'] =~ /darwin/
    s3_profile "default"
    # Get the credentials path and expand Windows environment variables
    default_path = File.join(ENV['USERPROFILE'] || ENV['HOME'] || '', '.aws', 'credentials')
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
    # Linux builders still rely on the EC2/pod instance profile.
    s3_instance_profile true
  end
end

# This setting can be overriden per-project (which is the case for the agent builds)
use_git_caching false

