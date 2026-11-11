# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "./lib/ostools.rb"

name 'agent-binaries'
package_name 'agent-binaries'
license "Apache-2.0"
license_file "../LICENSE"

homepage 'http://www.datadoghq.com'

install_dir ENV["INSTALL_DIR"] || raise('INSTALL_DIR must be set in tasks/omnibus.py')

if ohai['platform'] == "windows"
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Monitoring Agent
 The Datadog Monitoring Agent is a lightweight process that monitors system
 processes and services, and sends information back to your Datadog account.

 This package installs and runs the advanced Agent daemon, which queues and
 forwards metrics from your applications as well as system services.

 See http://www.datadoghq.com/ for more information
'

# ------------------------------------
# Generic package information
# ------------------------------------

# .msi specific flags
package :msi do
  skip_packager true
end
package :zip do
  extra_package_dirs [
      "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent\\extra_package_files",
      "#{Omnibus::Config.source_dir()}\\cf-root",
    ]
end

# Signed with windows-code-signer.exe just before packaging (see sign_file in
# lib/project_extension.rb), which uses the job's CI Identity AWS credentials.
# The ZIP packager's own signing (additional_sign_files + dd_wcssign) shells out
# to the legacy dd-wcs, which signs with the runner instance profile instead.
if ENV['SIGN_WINDOWS_DD_WCS']
  sign_file "#{Omnibus::Config.source_dir()}\\cf-root\\bin\\agent\\trace-agent.exe"
  sign_file "#{Omnibus::Config.source_dir()}\\cf-root\\bin\\agent\\dogstatsd.exe"
end


# ------------------------------------
# Dependencies
# ------------------------------------

# Datadog agent
dependency 'trace-agent-windows-cf'
dependency 'datadog-dogstatsd'

dependency 'datadog-buildpack-finalize'
exclude '\.git*'
exclude 'bundler\/git'
