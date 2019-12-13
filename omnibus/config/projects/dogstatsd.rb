# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

require "./lib/ostools.rb"

name 'dogstatsd'
package_name 'datadog-dogstatsd'
homepage 'http://www.datadoghq.com'
license "Apache-2.0"
license_file "../LICENSE"

if ohai['platform'] == "windows"
  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  install_dir "C:/opt/datadog-dogstatsd/"
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
  install_dir '/opt/datadog-dogstatsd'
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog dogstatsd agent
 Dogstatsd is a lightweight process that will receive dogstatsd packet, aggregate
 them and forward them to Datadog backend. Its main purpose is to handle custom
 metrics from external processes.
 .
 This package installs and runs Dogstatsd.
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


# Windows .msi specific flags
package :zip do
  skip_packager true
end

package :msi do

  # For a consistent package management, please NEVER change this code
  arch = "x64"
  if windows_arch_i386?
    upgrade_code 'a8c5b8ae-ac27-4d66-b63f-edba0e5ea477'
    arch = "x86"
  else
    upgrade_code 'dd60e9df-487b-415c-ba2f-dba19ddc7ebd'
  end
  wix_candle_extension 'WixUtilExtension'
  wix_light_extension 'WixUtilExtension'
  #extra_package_dir "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent\\extra_package_files"

  additional_sign_files [
      "#{Omnibus::Config.source_dir()}\\datadog-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\dogstatsd.exe"
    ]
  #if ENV['SIGN_WINDOWS']
  #  signing_identity "ECCDAE36FDCB654D2CBAB3E8975AA55469F96E4C", machine_store: true, algorithm: "SHA256"
  #end
  if ENV['SIGN_PFX']
    signing_identity_file "#{ENV['SIGN_PFX']}", password: "#{ENV['SIGN_PFX_PW']}", algorithm: "SHA256"
  end
  parameters({
    'InstallDir' => install_dir,
    'InstallFiles' => "#{Omnibus::Config.source_dir()}/datadog-agent/dd-agent/packaging/datadog-agent/win32/install_files",
    'BinFiles' => "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent",
    'EtcFiles' => "#{Omnibus::Config.source_dir()}\\etc\\datadog-dogstatsd",
    'Platform' => "#{arch}",
  })
end
# OSX .pkg specific flags
package :pkg do
  identifier 'com.datadoghq.dogstatsd'
  #signing_identity 'Developer ID Installer: Datadog, Inc. (JKFCB4CN7C)'
end
compress :dmg do
  window_bounds '200, 200, 750, 600'
  pkg_position '10, 10'
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'datadog-agent-prepare'

# version manifest file
dependency 'version-manifest'

# Dogstatsd
dependency 'datadog-dogstatsd'

# this dependency puts few files out of the omnibus install dir and move them
# in the final destination. This way such files will be listed in the packages
# manifest and owned by the package manager. This is the only point in the build
# process where we operate outside the omnibus install dir, thus the need of
# the `extra_package_file` directive.
# This must be the last dependency in the project.

dependency 'datadog-dogstatsd-finalize'

if linux?
  extra_package_file '/etc/init/datadog-dogstatsd.conf'
  extra_package_file '/lib/systemd/system/datadog-dogstatsd.service'
  extra_package_file '/etc/datadog-dogstatsd/'
end

exclude '\.git*'
exclude 'bundler\/git'
