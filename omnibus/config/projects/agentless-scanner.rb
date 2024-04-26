# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "./lib/ostools.rb"

name 'agentless-scanner'
package_name 'datadog-agentless-scanner'
homepage 'http://www.datadoghq.com'
license "Apache-2.0"
license_file "../LICENSE"

if ENV.has_key?("OMNIBUS_WORKERS_OVERRIDE")
  COMPRESSION_THREADS = ENV["OMNIBUS_WORKERS_OVERRIDE"].to_i
else
  COMPRESSION_THREADS = 1
end
if ENV.has_key?("DEPLOY_AGENT") && ENV["DEPLOY_AGENT"] == "true"
  COMPRESSION_LEVEL = 9
else
  COMPRESSION_LEVEL = 5
end

install_dir '/opt/datadog/agentless-scanner'
if redhat_target? || suse_target?
  maintainer 'Datadog, Inc <package@datadoghq.com>'

  # NOTE: with script dependencies, we only care about preinst/postinst/posttrans,
  # because these would be used in a kickstart during package installation phase.
  # All of the packages that we depend on in prerm/postrm scripts always have to be
  # installed on all distros that we support, so we don't have to depend on them
  # explicitly.

  # postinst and posttrans scripts use a subset of preinst script deps, so we don't
  # have to list them, because they'll already be there because of preinst
  runtime_script_dependency :pre, "coreutils"
  runtime_script_dependency :pre, "grep"
  if redhat_target?
    runtime_script_dependency :pre, "glibc-common"
    runtime_script_dependency :pre, "shadow-utils"
  else
    runtime_script_dependency :pre, "glibc"
    runtime_script_dependency :pre, "shadow"
  end
else
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

if debian_target?
  runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.3.1)'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Agentless Scanner
 The Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.
 .
 This package installs and runs the Agentless Scanner.
 .
 See http://www.datadoghq.com/ for more information
'

# ------------------------------------
# Generic package information
# ------------------------------------

# .deb specific flags
package :deb do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  license 'Apache License Version 2.0'
  section 'utils'
  priority 'extra'
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
  compression_algo "xz"
  if ENV.has_key?('DEB_SIGNING_PASSPHRASE') and not ENV['DEB_SIGNING_PASSPHRASE'].empty?
    signing_passphrase "#{ENV['DEB_SIGNING_PASSPHRASE']}"
    if ENV.has_key?('DEB_GPG_KEY_NAME') and not ENV['DEB_GPG_KEY_NAME'].empty?
      gpg_key_name "#{ENV['DEB_GPG_KEY_NAME']}"
    end
  end
end

# .rpm specific flags
package :rpm do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  dist_tag ''
  license 'Apache License Version 2.0'
  category 'System Environment/Daemons'
  priority 'extra'
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
  compression_algo "xz"
  if ENV.has_key?('RPM_SIGNING_PASSPHRASE') and not ENV['RPM_SIGNING_PASSPHRASE'].empty?
    signing_passphrase "#{ENV['RPM_SIGNING_PASSPHRASE']}"
    if ENV.has_key?('RPM_GPG_KEY_NAME') and not ENV['RPM_GPG_KEY_NAME'].empty?
      gpg_key_name "#{ENV['RPM_GPG_KEY_NAME']}"
    end
  end
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'datadog-agent-prepare'

# version manifest file
dependency 'version-manifest'

# Agentless-scanner
dependency 'datadog-agentless-scanner'

# this dependency puts few files out of the omnibus install dir and move them
# in the final destination. This way such files will be listed in the packages
# manifest and owned by the package manager. This is the only point in the build
# process where we operate outside the omnibus install dir, thus the need of
# the `extra_package_file` directive.
# This must be the last dependency in the project.

dependency 'datadog-agentless-scanner-finalize'

# package scripts
if linux_target?
  if debian_target?
    package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/agentless-scanner-deb"
  else
    package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/agentless-scanner-rpm"
  end
end

if linux_target?
  extra_package_file '/lib/systemd/system/datadog-agentless-scanner.service'
  extra_package_file '/var/log/datadog/'
end

exclude '\.git*'
exclude 'bundler\/git'
