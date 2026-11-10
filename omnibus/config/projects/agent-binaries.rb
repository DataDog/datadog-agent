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

if ohai['platform'] == "windows"
  # Note: this is not the final install dir, not even the default one, just a convenient
  # spaceless dir in which the agent will be built.
  # Omnibus doesn't quote the Git commands it launches unfortunately, which makes it impossible
  # to put a space here...
  install_dir "C:/opt/datadog-agent/"
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
  install_dir '/opt/datadog-agent'
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Monitoring Agent
 The Datadog Monitoring Agent is a lightweight process that monitors system
 processes and services, and sends information back to your Datadog account.
 .
 This package installs and runs the advanced Agent daemon, which queues and
 forwards metrics from your applications as well as system services.
 .
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

  if ENV['SIGN_PFX']
    signing_identity_file "#{ENV['SIGN_PFX']}", password: "#{ENV['SIGN_PFX_PW']}", algorithm: "SHA256"
  end
end

# Signed with windows-code-signer.exe just before packaging (see sign_file in
# lib/project_extension.rb), which uses the job's CI Identity AWS credentials.
# The ZIP packager's own signing (additional_sign_files + dd_wcssign) shells out
# to the legacy dd-wcs, which signs with the runner instance profile instead.
if ENV['SIGN_WINDOWS_DD_WCS']
  sign_file "#{Omnibus::Config.source_dir()}\\cf-root\\bin\\agent\\process-agent.exe"
  sign_file "#{Omnibus::Config.source_dir()}\\cf-root\\bin\\agent\\trace-agent.exe"
  sign_file "#{Omnibus::Config.source_dir()}\\cf-root\\bin\\agent\\dogstatsd.exe"
  sign_file "#{Omnibus::Config.source_dir()}\\cf-root\\bin\\agent.exe"
end


# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'preparation'

# Datadog agent
dependency 'datadog-iot-agent'
dependency 'datadog-dogstatsd'

# version manifest file
dependency 'version-manifest'

dependency 'datadog-buildpack-finalize'
exclude '\.git*'
exclude 'bundler\/git'
