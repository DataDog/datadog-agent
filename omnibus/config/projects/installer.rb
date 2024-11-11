# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"

name 'installer'
package_name 'datadog-installer'
license "Apache-2.0"
license_file "../LICENSE"

third_party_licenses "../LICENSE-3rdparty.csv"

homepage 'http://www.datadoghq.com'

if windows_target?
  INSTALL_DIR = 'C:/opt/datadog-installer/'
else
  INSTALL_DIR = ENV['INSTALL_DIR'] || '/opt/datadog-installer'
end

install_dir INSTALL_DIR

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

if windows_target?
  maintainer 'Datadog Inc.'
elsif redhat_target? || suse_target?
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

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Installer
 The Datadog Installer is a lightweight process that installs and updates the Datadog Agent
 and Tracers.

 See http://www.datadoghq.com/ for more information
'

if ENV["OMNIBUS_PACKAGE_ARTIFACT_DIR"]
  dependency "package-artifact"
  generate_distro_package = true
else
  # creates required build directories
  dependency 'preparation'

  dependency 'installer'

  # version manifest file
  dependency 'version-manifest'
  generate_distro_package = false
end


# ------------------------------------
# Generic package information
# ------------------------------------

# .deb specific flags
package :deb do
  skip_packager !generate_distro_package
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

package :rpm do
  skip_packager !generate_distro_package
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

package :msi do
  skip_packager true
end

package :xz do
  skip_packager generate_distro_package
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
end

# ------------------------------------
# Dependencies
# ------------------------------------

# Include all package scripts for the intermediary XZ package, or only those
# for the package being created
if linux_target?
  if !generate_distro_package
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/installer-deb"
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/installer-rpm"
    extra_package_file "#{Omnibus::Config.project_root}/config/templates/installer/README.md.erb"
  end
  if debian_target?
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/installer-deb"
  elsif redhat_target? || suse_target?
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/installer-rpm"
  end
end

exclude '\.git*'
exclude 'bundler\/git'

if windows_target?
  BINARIES = [
    "#{install_dir}\\datadog-installer.exe"
  ]
  BINARIES.each do |bin|
    windows_symbol_stripping_file bin
    if ENV['SIGN_WINDOWS_DD_WCS']
      sign_file bin
    end
  end
end

if linux_target? or windows_target?
  # the stripper will drop the symbols in a `.debug` folder in the installdir
  # we want to make sure that directory is not in the main build, while present
  # in the debug package.
  strip_build !generate_distro_package
  debug_path ".debug"  # the strip symbols will be in here
end
