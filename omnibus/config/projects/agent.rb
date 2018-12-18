# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require "./lib/ostools.rb"

name 'agent'
package_name 'datadog-agent'

homepage 'http://www.datadoghq.com'

if ohai['platform'] == "windows"
  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  install_dir "C:/opt/datadog-agent/"
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
  install_dir '/opt/datadog-agent'
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

build_version do
  source :git
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
  license 'Apache License Version 2.0'
  section 'utils'
  priority 'extra'
end

# .rpm specific flags
package :rpm do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  dist_tag ''
  license 'Apache License Version 2.0'
  category 'System Environment/Daemons'
  priority 'extra'
  if ENV.has_key?('RPM_SIGNING_PASSPHRASE') and not ENV['RPM_SIGNING_PASSPHRASE'].empty?
    signing_passphrase "#{ENV['RPM_SIGNING_PASSPHRASE']}"
  end
end

# OSX .pkg specific flags
package :pkg do
  identifier 'com.datadoghq.agent'
  unless ENV['SKIP_SIGN_MAC'] == 'true'
    signing_identity 'Developer ID Installer: Datadog, Inc. (JKFCB4CN7C)'
  end
end
compress :dmg do
  window_bounds '200, 200, 584, 632'

  pkg_position '10, 10'
end

# Windows .msi specific flags
package :msi do

  # For a consistent package management, please NEVER change this code
  upgrade_code '0c50421b-aefb-4f15-a809-7af256d608a5'
  wix_candle_extension 'WixUtilExtension'
  wix_light_extension 'WixUtilExtension'
  extra_package_dir "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent\\extra_package_files"
  additional_sign_files [
      "#{Omnibus::Config.source_dir()}\\datadog-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\trace-agent.exe",
      "#{Omnibus::Config.source_dir()}\\datadog-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\agent.exe"
    ]
  if ENV['SIGN_WINDOWS']
    signing_identity "ECCDAE36FDCB654D2CBAB3E8975AA55469F96E4C", machine_store: true, algorithm: "SHA256"
  end
  parameters({
    'InstallDir' => install_dir,
    'InstallFiles' => "#{Omnibus::Config.source_dir()}/datadog-agent/dd-agent/packaging/datadog-agent/win32/install_files",
    'BinFiles' => "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent",
    'EtcFiles' => "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent",
  })
end

# ------------------------------------
# Dependencies
# ------------------------------------

# Linux-specific dependencies
if linux?
  dependency 'procps-ng'
  dependency 'sysstat'
  dependency 'curl'
end

dependency 'cacerts'
# creates required build directories
dependency 'datadog-agent-prepare'

# Datadog agent
dependency 'datadog-agent'

# Additional software
dependency 'datadog-pip'
dependency 'datadog-agent-integrations'
dependency 'jmxfetch'

# External agents
dependency 'datadog-trace-agent'
dependency 'datadog-process-agent' # Includes network-tracer

if osx?
  dependency 'datadog-agent-mac-app'
end

# Remove pyc/pyo files from package
# should be built after all the other python-related software defs
unless osx?
  dependency 'py-compiled-cleanup'
end

# version manifest file
dependency 'version-manifest'

# this dependency puts few files out of the omnibus install dir and move them
# in the final destination. This way such files will be listed in the packages
# manifest and owned by the package manager. This is the only point in the build
# process where we operate outside the omnibus install dir, thus the need of
# the `extra_package_file` directive.
# This must be the last dependency in the project.

dependency 'datadog-agent-finalize'

if linux?
  extra_package_file '/etc/init/datadog-agent.conf'
  extra_package_file '/etc/init/datadog-agent-process.conf'
  extra_package_file '/etc/init/datadog-agent-network.conf'
  extra_package_file '/etc/init/datadog-agent-trace.conf'
  systemd_directory = "/usr/lib/systemd/system"
  if debian?
    systemd_directory = "/lib/systemd/system"

    extra_package_file "/etc/init.d/datadog-agent"
    extra_package_file "/etc/init.d/datadog-agent-process"
    extra_package_file "/etc/init.d/datadog-agent-trace"
  end
  extra_package_file "#{systemd_directory}/datadog-agent.service"
  extra_package_file "#{systemd_directory}/datadog-agent-process.service"
  extra_package_file "#{systemd_directory}/datadog-agent-network.service"
  extra_package_file "#{systemd_directory}/datadog-agent-trace.service"
  extra_package_file '/etc/datadog-agent/'
  extra_package_file '/usr/bin/dd-agent'
  extra_package_file '/var/log/datadog/'
end

exclude '\.git*'
exclude 'bundler\/git'
