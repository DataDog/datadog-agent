# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"
flavor = ENV['AGENT_FLAVOR']
output_config_dir = ENV["OUTPUT_CONFIG_DIR"]

if flavor.nil? || flavor == 'base'
  name 'agent'
  package_name 'datadog-agent'
else
  name "agent-#{flavor}"
  package_name "datadog-#{flavor}-agent"
end
license "Apache-2.0"
license_file "../LICENSE"

third_party_licenses "../LICENSE-3rdparty.csv"

homepage 'http://www.datadoghq.com'

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

BUILD_OCIRU = Omnibus::Config.host_distribution == "ociru"

if ENV.has_key?("OMNIBUS_GIT_CACHE_DIR") && !BUILD_OCIRU
  Omnibus::Config.use_git_caching true
  Omnibus::Config.git_cache_dir ENV["OMNIBUS_GIT_CACHE_DIR"]
end

if windows_target?
  if ot_target?
    raise UnknownPlatform
  end

  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  INSTALL_DIR = 'C:/opt/datadog-agent/'
  PYTHON_3_EMBEDDED_DIR = format('%s/embedded3', INSTALL_DIR)
else
  INSTALL_DIR = ENV["INSTALL_DIR"] || '/opt/datadog-agent'
end

install_dir INSTALL_DIR

if windows_target?
  python_3_embedded PYTHON_3_EMBEDDED_DIR
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
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
    runtime_script_dependency :pre, "findutils"
    runtime_script_dependency :pre, "grep"
    if redhat_target?
      runtime_script_dependency :pre, "glibc-common"
      runtime_script_dependency :pre, "shadow-utils"
      conflict "glibc-common < 2.17"
    else
      runtime_script_dependency :pre, "glibc"
      runtime_script_dependency :pre, "shadow"
    end
  else
    maintainer 'Datadog Packages <package@datadoghq.com>'
  end

  if debian_target?
    runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.4.0)'
  end

  if osx_target?
    unless ENV['SKIP_SIGN_MAC'] == 'true'
      code_signing_identity 'Developer ID Application: Datadog, Inc. (JKFCB4CN7C)'
    end
    if ENV['HARDENED_RUNTIME_MAC'] == 'true'
      entitlements_file "#{files_path}/macos/Entitlements.plist"
    end
  else
    conflict 'datadog-iot-agent'
  end
end

do_build = false
do_package = false

if ENV["OMNIBUS_PACKAGE_ARTIFACT_DIR"]
  dependency "package-artifact"
  do_package = true
  skip_healthcheck true
else
  do_build = true
end

# For now we build and package in the same stage for heroku
if heroku_target?
  do_build = true
  do_package = true
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

# .deb specific flags
package :deb do
  skip_packager !do_package
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
  skip_packager !do_package
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

# OSX .pkg specific flags
package :pkg do
  skip_packager BUILD_OCIRU
  identifier 'com.datadoghq.agent'
  unless ENV['SKIP_SIGN_MAC'] == 'true'
    signing_identity 'Developer ID Installer: Datadog, Inc. (JKFCB4CN7C)'
  end
end
compress :dmg do
  window_bounds '200, 200, 584, 632'

  pkg_position '10, 10'
end

# Windows .zip specific flags
package :zip do
  if windows_arch_i386?
    skip_packager true
  else
    # noinspection RubyLiteralArrayInspection
    extra_package_dirs [
      "#{Omnibus::Config.source_dir()}\\etc\\datadog-agent\\extra_package_files",
      "#{Omnibus::Config.source_dir()}\\cf-root"
    ]
  end
end

package :msi do
  skip_packager true
end

package :xz do
  skip_packager (!do_build && !BUILD_OCIRU)
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
end

# ------------------------------------
# Dependencies
# ------------------------------------

if do_build
  # Datadog agent
  dependency 'datadog-agent'

  # System-probe
  if linux_target? && !heroku_target?
    dependency 'system-probe'
  end

  if osx_target?
    dependency 'datadog-agent-mac-app'
  end

  dependency 'datadog-agent-integrations-py3'

  if linux_target?
    dependency 'datadog-security-agent-policies'
  end

  # Include traps db file in snmp.d/traps_db/
  dependency 'snmp-traps'

  # Additional software
  if windows_target?
    if ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?
      dependency 'datadog-windows-filter-driver'
    end
    if ENV['WINDOWS_APMINJECT_MODULE'] and not ENV['WINDOWS_APMINJECT_MODULE'].empty?
      dependency 'datadog-windows-apminject'
    end
    if ENV['WINDOWS_DDPROCMON_DRIVER'] and not ENV['WINDOWS_DDPROCMON_DRIVER'].empty?
      dependency 'datadog-windows-procmon-driver'
      ## this is a duplicate of the above dependency in linux
      dependency 'datadog-security-agent-policies'
    end
  end

  # this dependency puts few files out of the omnibus install dir and move them
  # in the final destination. This way such files will be listed in the packages
  # manifest and owned by the package manager. This is the only point in the build
  # process where we operate outside the omnibus install dir, thus the need of
  # the `extra_package_file` directive.
  # This must be the last dependency in the project.
  dependency 'datadog-agent-finalize'
  dependency 'datadog-cf-finalize'
  # Special csae for heroku which does build & packaging in a single step
  if do_package
    dependency "init-scripts-agent"
  end
elsif do_package
  dependency "package-artifact"
  dependency "init-scripts-agent"
end

if linux_target?
  extra_package_file "#{output_config_dir}/etc/datadog-agent/"
  extra_package_file '/usr/bin/dd-agent'
  extra_package_file '/var/log/datadog/'
end

# all flavors use the same package scripts
if linux_target?
  if do_build && !do_package
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/agent-deb"
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/agent-rpm"
  end
  if do_package
    if debian_target?
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/agent-deb"
    else
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/agent-rpm"
    end
  end
elsif osx_target?
    package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/agent-dmg"
end

resources_path "#{Omnibus::Config.project_root}/resources/agent"

exclude '\.git*'
exclude 'bundler\/git'

if windows_target?
  FORBIDDEN_SYMBOLS = [
    "github.com/golang/glog"
  ]

  raise_if_forbidden_symbol_found = Proc.new { |symbols|
    FORBIDDEN_SYMBOLS.each do |fs|
      count = symbols.scan(fs).count()
      if count > 0
        raise ForbiddenSymbolsFoundError.new("#{fs} should not be present in the Agent binary but #{count} was found")
      end
    end
  }

  GO_BINARIES = [
    "#{install_dir}\\bin\\agent\\agent.exe",
    "#{install_dir}\\bin\\agent\\trace-agent.exe",
    "#{install_dir}\\bin\\agent\\process-agent.exe",
    "#{install_dir}\\bin\\agent\\system-probe.exe"
  ]
  if not windows_arch_i386? and ENV['WINDOWS_DDPROCMON_DRIVER'] and not ENV['WINDOWS_DDPROCMON_DRIVER'].empty?
    GO_BINARIES << "#{install_dir}\\bin\\agent\\security-agent.exe"
  end

  GO_BINARIES.each do |bin|
    # Check the exported symbols from the binary
    inspect_binary(bin, &raise_if_forbidden_symbol_found)

    # strip the binary of debug symbols
    windows_symbol_stripping_file bin
  end

  if ENV['SIGN_WINDOWS_DD_WCS']
    BINARIES_TO_SIGN = GO_BINARIES + [
      "#{install_dir}\\bin\\agent\\ddtray.exe",
      "#{install_dir}\\bin\\agent\\libdatadog-agent-three.dll"
    ]

    BINARIES_TO_SIGN.each do |bin|
      sign_file bin
    end
  end

end

if linux_target? or windows_target?
  # the stripper will drop the symbols in a `.debug` folder in the installdir
  # we want to make sure that directory is not in the main build, while present
  # in the debug package.
  strip_build windows_target? || do_build
  debug_path ".debug"  # the strip symbols will be in here
end
