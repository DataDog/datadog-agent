# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

require "./lib/ostools.rb"

name 'puppy'
package_name 'stackstate-puppy'

homepage 'http://www.stackstate.com'

if ohai['platform'] == "windows"
  # Note: this is not the final install dir, not even the default one, just a convenient
  # spaceless dir in which the agent will be built.
  # Omnibus doesn't quote the Git commands it launches unfortunately, which makes it impossible
  # to put a space here...
  install_dir "C:/opt/stackstate-agent/"
  maintainer 'StackState Inc.' # Windows doesn't want our e-mail address :(
else
  install_dir '/opt/stackstate-agent6'
  maintainer 'StackState <info@stackstate.com>'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'StackState Monitoring Agent
 The StackState Monitoring Agent is a lightweight process that monitors system
 processes and services, and sends information back to your Datadog account.
 .
 This package installs and runs the advanced Agent daemon, which queues and
 forwards metrics from your applications as well as system services.
 .
 See http://www.stackstate.com for more information
'

# ------------------------------------
# Generic package information
# ------------------------------------

# .deb specific flags
package :deb do
  vendor 'StackState <info@stackstate.com>'
  epoch 1
  license 'Simplified BSD License'
  section 'utils'
  priority 'extra'
end

# ------------------------------------
# OS specific DSLs and dependencies
# ------------------------------------

# Linux
if linux?
  if debian?
    extra_package_file '/etc/init/stackstate-agent6.conf'
    extra_package_file '/lib/systemd/system/stackstate-agent6.service'
  end

  # Example configuration files for the agent and the checks
  extra_package_file '/etc/stackstate-agent/stackstate.yaml.example'
  extra_package_file '/etc/stackstate-agent/conf.d/'

  # Logs directory
  extra_package_file '/var/log/datadog/'
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'preparation'

# Datadog agent
dependency 'datadog-puppy'

# version manifest file
dependency 'version-manifest'

exclude '\.git*'
exclude 'bundler\/git'
