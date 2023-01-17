# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "./lib/ostools.rb"

name 'iot-agent'
package_name 'datadog-iot-agent'
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
  if redhat? || suse?
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
    if redhat?
      runtime_script_dependency :pre, "glibc-common"
      runtime_script_dependency :pre, "shadow-utils"
    else
      runtime_script_dependency :pre, "glibc"
      runtime_script_dependency :pre, "shadow"
    end
  else
    maintainer 'Datadog Packages <package@datadoghq.com>'
  end

  if debian?
    runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.1.0)'
  end
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog IoT Agent
 The Datadog IoT Agent is a lightweight process that monitors system
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
  if ENV.has_key?('RPM_SIGNING_PASSPHRASE') and not ENV['RPM_SIGNING_PASSPHRASE'].empty?
    signing_passphrase "#{ENV['RPM_SIGNING_PASSPHRASE']}"
    if ENV.has_key?('RPM_GPG_KEY_NAME') and not ENV['RPM_GPG_KEY_NAME'].empty?
      gpg_key_name "#{ENV['RPM_GPG_KEY_NAME']}"
    end
  end
end

# ------------------------------------
# OS specific DSLs and dependencies
# ------------------------------------

# Linux
if linux?
  # Upstart
  if debian? || redhat? || suse?
    extra_package_file '/etc/init/datadog-agent.conf'
  end

  # Systemd
  if debian?
    extra_package_file '/lib/systemd/system/datadog-agent.service'
  else
    extra_package_file '/usr/lib/systemd/system/datadog-agent.service'
  end

  # Example configuration files for the agent and the checks
  extra_package_file '/etc/datadog-agent/datadog.yaml.example'
  extra_package_file '/etc/datadog-agent/conf.d/'

  # Logs directory
  extra_package_file '/var/log/datadog/'
end

# Windows .msi specific flags
package :zip do
  skip_packager true
end

package :msi do

  # For a consistent package management, please NEVER change this code
  arch = "x64"
  if windows_arch_i386?
    full_agent_upgrade_code = '2497f989-f07e-4e8c-9e05-841ad3d4405f'
    upgrade_code '6f7ac237-334c-44c8-9fec-ec8f3459db37'
    arch = "x86"
  else
    full_agent_upgrade_code = '0c50421b-aefb-4f15-a809-7af256d608a5'
    upgrade_code '1b3d4067-fd27-4de4-bfc9-605695ad514c'
  end
  wix_candle_extension 'WixUtilExtension'
  wix_light_extension 'WixUtilExtension'
  extra_package_dir "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent\\extra_package_files"

  additional_sign_files [
      "#{Omnibus::Config.source_dir()}\\datadog-iot-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\security-agent.exe",
      "#{Omnibus::Config.source_dir()}\\datadog-iot-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\process-agent.exe",
      "#{Omnibus::Config.source_dir()}\\datadog-iot-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\trace-agent.exe",
      "#{Omnibus::Config.source_dir()}\\datadog-iot-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\agent.exe"
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
    'BinFiles' => "#{Omnibus::Config.source_dir()}/datadog-iot-agent/src/github.com/DataDog/datadog-agent/bin/agent",
    'EtcFiles' => "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent",
    'Platform' => "#{arch}",
  })
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'preparation'

# Datadog agent
dependency 'datadog-iot-agent'

if windows?
  dependency 'datadog-agent-finalize'
end

# version manifest file
dependency 'version-manifest'

# package scripts
if linux?
  if debian?
    package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/iot-agent-deb"
  else
    package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/iot-agent-rpm"
  end
end

exclude '\.git*'
exclude 'bundler\/git'
