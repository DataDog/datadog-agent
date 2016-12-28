#
# Copyright 2016 Datadog
#
# All Rights Reserved.
#
require "./lib/ostools.rb"

name 'datadog-agent6'
maintainer 'Datadog Packages <package@datadoghq.com>'
homepage 'http://www.datadoghq.com'
install_dir '/opt/datadog-agent6'

build_version do
  source :git, from_dependency: 'datadog-agent'
  output_format :dd_agent_format
end

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

# .deb specific flags
package :deb do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  license 'Simplified BSD License'
  section 'utils'
  priority 'extra'
end

# .rpm specific flags
package :rpm do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  license 'Simplified BSD License'
  category 'System Environment/Daemons'
  priority 'extra'
  if ENV.has_key?('RPM_SIGNING_PASSPHRASE') and not ENV['RPM_SIGNING_PASSPHRASE'].empty?
    signing_passphrase "#{ENV['RPM_SIGNING_PASSPHRASE']}"
  end
end

# OSX .pkg specific flags
package :pkg do
  identifier 'com.datadoghq.agent'
  #signing_identity 'Developer ID Installer: Datadog, Inc. (JKFCB4CN7C)'
end
compress :dmg do
  window_bounds '200, 200, 750, 600'
  pkg_position '10, 10'
end

# ------------------------------------
# OS specific DSLs and dependencies
# ------------------------------------

# Linux
if linux?
  if debian?
    extra_package_file '/etc/init/datadog-agent6.conf'
    extra_package_file '/lib/systemd/system/datadog-agent6.service'
  end

  # Linux-specific dependencies
  dependency 'procps-ng'
  dependency 'sysstat'
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'preparation'

# Agent dependencies

# Check dependencies
dependency 'requests'
dependency 'psutil'

# Additional software
dependency 'datadogpy'

# Datadog agent
dependency 'datadog-agent'

# version manifest file
dependency 'version-manifest'

exclude '\.git*'
exclude 'bundler\/git'
