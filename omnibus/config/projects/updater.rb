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

if ENV.has_key?("OMNIBUS_WORKERS_OVERRIDE")
  COMPRESSION_THREADS = ENV["OMNIBUS_WORKERS_OVERRIDE"].to_i
else
  COMPRESSION_THREADS = 1
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
  skip_packager true
end

package :xz do
  skip_packager false
  compression_threads COMPRESSION_THREADS
  compression_level 5
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
