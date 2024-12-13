# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "./lib/ostools.rb"

name 'dogstatsd'
package_name 'datadog-dogstatsd'
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

if ohai['platform'] == "windows"
  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  install_dir "C:/opt/datadog-dogstatsd/"
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
  install_dir ENV["INSTALL_DIR"] || '/opt/datadog-dogstatsd'
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
    runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.4.0)'
  end
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

if ENV["OMNIBUS_PACKAGE_ARTIFACT_DIR"]
  dependency "package-artifact"
  do_package = true
else
  do_package = false
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
end


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


# Windows .msi specific flags
package :zip do
  skip_packager true
end

package :xz do
  skip_packager do_package
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
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

  additional_sign_files [
      "#{Omnibus::Config.source_dir()}\\datadog-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\dogstatsd.exe"
    ]
  if ENV['SIGN_WINDOWS_DD_WCS']
    dd_wcssign true
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

# package scripts
if linux_target?
  if !do_package
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/dogstatsd-deb"
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/dogstatsd-rpm"
  else
    if debian_target?
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/dogstatsd-deb"
    else
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/dogstatsd-rpm"
    end
  end
end

if linux_target?
  extra_package_file '/etc/init/datadog-dogstatsd.conf'
  extra_package_file '/lib/systemd/system/datadog-dogstatsd.service'
  extra_package_file '/etc/datadog-dogstatsd/'
  extra_package_file '/var/log/datadog/'
end

exclude '\.git*'
exclude 'bundler\/git'
