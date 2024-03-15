# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"

name 'updater'
package_name 'datadog-updater'
license "Apache-2.0"
license_file "../LICENSE"

third_party_licenses "../LICENSE-3rdparty.csv"

homepage 'http://www.datadoghq.com'

INSTALL_DIR = '/opt/datadog/updater'

install_dir INSTALL_DIR

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
  else
    runtime_script_dependency :pre, "glibc"
    runtime_script_dependency :pre, "shadow"
  end
else
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

if debian_target?
  runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.3.1)'
end

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Updater
 The Datadog Updater is a lightweight process that updates the Datadog Agent
 and Tracers.

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

package :xz do
  skip_packager true
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'preparation'

dependency 'updater'

# version manifest file
dependency 'version-manifest'

if linux_target?
  systemd_directory = "/usr/lib/systemd/system"
  if debian_target?
    systemd_directory = "/lib/systemd/system"
  end
  extra_package_file "#{systemd_directory}/datadog-updater.service"
  extra_package_file "#{systemd_directory}/datadog-agent.service"
  extra_package_file "#{systemd_directory}/datadog-agent-exp.service"
  extra_package_file "#{systemd_directory}/datadog-agent-trace.service"
  extra_package_file "#{systemd_directory}/datadog-agent-trace-exp.service"
  extra_package_file "#{systemd_directory}/datadog-agent-process.service"
  extra_package_file "#{systemd_directory}/datadog-agent-process-exp.service"
  extra_package_file "#{systemd_directory}/datadog-agent-security.service"
  extra_package_file "#{systemd_directory}/datadog-agent-security-exp.service"
  extra_package_file "#{systemd_directory}/datadog-agent-sysprobe.service"
  extra_package_file "#{systemd_directory}/datadog-agent-sysprobe-exp.service"
  extra_package_file "#{systemd_directory}/datadog-agent.path"
  extra_package_file "#{systemd_directory}/datadog-agent-exp.path"
  extra_package_file '/etc/datadog-agent/'
  extra_package_file '/var/log/datadog/'
  extra_package_file '/var/run/datadog-packages/'
  extra_package_file '/opt/datadog-packages/'
end

if linux_target?
  if debian_target?
    package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/updater-deb"
  end
end

exclude '\.git*'
exclude 'bundler\/git'

if linux_target?
  # the stripper will drop the symbols in a `.debug` folder in the installdir
  # we want to make sure that directory is not in the main build, while present
  # in the debug package.
  strip_build true
  debug_path ".debug"  # the strip symbols will be in here
end
