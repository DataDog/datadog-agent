# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

require "./lib/ostools.rb"

name 'agent'
package_name 'stackstate-agent'

homepage 'http://www.stackstate.com'

if ohai['platform'] == "windows"
  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  install_dir "C:/opt/stackstate-agent/"
  maintainer 'StackState B.V.' # Windows doesn't want our e-mail address :(
else
  install_dir '/opt/stackstate-agent'
  maintainer 'StackState <info@stackstate.com>'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'StackState Monitoring Agent
 The StackState Monitoring Agent is a lightweight process that monitors system
 processes and services, and sends information back to your StackState account.
 .
 This package installs and runs the advanced Agent daemon, which queues and
 forwards metrics from your applications as well as system services.
 .
 See http://www.stackstate.com/ for more information
'

# ------------------------------------
# Generic package information
# ------------------------------------

# .deb specific flags
package :deb do
  vendor 'StackState <info@stackstate.com>'
  license 'Apache License Version 2.0'
  section 'utils'
  priority 'extra'
end

# .rpm specific flags
package :rpm do
  vendor 'StackState <info@stackstate.com>'
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
  extra_package_dir "#{Omnibus::Config.source_dir()}\\etc\\stackstate-agent\\extra_package_files"
  additional_sign_files [
      "#{Omnibus::Config.source_dir()}\\stackstate-agent\\src\\github.com\\StackVista\\stackstate-agent\\bin\\agent\\trace-agent.exe",
      "#{Omnibus::Config.source_dir()}\\stackstate-agent\\src\\github.com\\StackVista\\stackstate-agent\\bin\\agent\\agent.exe"
    ]
  if ENV['SIGN_WINDOWS']
    signing_identity "ECCDAE36FDCB654D2CBAB3E8975AA55469F96E4C", machine_store: true, algorithm: "SHA256"
  end
  parameters({
    'InstallDir' => install_dir,
    'InstallFiles' => "#{Omnibus::Config.source_dir()}/stackstate-agent/stackstate-agent/packaging/stackstate-agent/win32/install_files",
    'BinFiles' => "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/StackVista/stackstate-agent/bin/agent",
    'EtcFiles' => "#{Omnibus::Config.source_dir()}\\etc\\stackstate-agent",
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

# [VS] missing in upstream
# # Windows-specific dependencies
# if windows?
#   dependency 'pywin32'
# end

dependency 'datadog-agent'

# Additional software
dependency 'pip'
dependency 'stackstate-agent-integrations'
dependency 'datadog-a7'
dependency 'datadog-agent-env-check'
dependency 'jmxfetch'

# External agents
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
  extra_package_file '/etc/init/stackstate-agent.conf'
  extra_package_file '/etc/init/stackstate-agent-process.conf'
  #[VS] extra_package_file '/etc/init/stackstate-agent-network.conf'
  extra_package_file '/etc/init/stackstate-agent-trace.conf'
  systemd_directory = "/usr/lib/systemd/system"
  if debian?
    systemd_directory = "/lib/systemd/system"


    extra_package_file "/etc/init.d/stackstate-agent"
    extra_package_file "/etc/init.d/stackstate-agent-process"
    extra_package_file "/etc/init.d/stackstate-agent-trace"
  end
  extra_package_file "#{systemd_directory}/stackstate-agent.service"
  extra_package_file "#{systemd_directory}/stackstate-agent-process.service"
  #[VS] extra_package_file "#{systemd_directory}/stackstate-agent-network.service"
  extra_package_file "#{systemd_directory}/stackstate-agent-trace.service"
  extra_package_file '/etc/stackstate-agent/'
  extra_package_file '/usr/bin/sts-agent'
  extra_package_file '/var/log/stackstate-agent/'
end

exclude '\.git*'
exclude 'bundler\/git'
