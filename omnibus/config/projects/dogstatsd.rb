#
# Copyright 2016 Datadog
#
# All Rights Reserved.
#
require "./lib/ostools.rb"

# --------------------------------------------------
# WIP / FIXME:
# The dogstatsd package is currently not working as
# a standalone package. Work will be put into making
# it fully contained and stable in the future.
# --------------------------------------------------
name 'dogstatsd'
maintainer 'Datadog Packages <package@datadoghq.com>'
homepage 'http://www.datadoghq.com'
install_dir '/opt/datadog-dogstatsd'

build_version do
  source :git, from_dependency: 'dogstatsd'
  output_format :dd_agent_format
end

build_iteration 1

description 'Datadog dogstatsd agent
 The Datadog dogstatsd agent is a lightweight process that will receive
 dogstatsd packet, aggregate them and forward them to Datadog backend. The main
 purpose of the agent is to handle custom metrics from external processes.
 .
 This package installs and runs the dogstatsd agent.
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
  dist_tag ''
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
    extra_package_file '/etc/init/dogstatsd.conf'
    extra_package_file '/lib/systemd/system/dogstatsd.service'
  end
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'preparation'

# version manifest file
dependency 'version-manifest'

# Dogstatsd
dependency 'dogstatsd'

exclude '\.git*'
exclude 'bundler\/git'
