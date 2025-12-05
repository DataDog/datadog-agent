# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"
require "./lib/project_helpers.rb"
require "./lib/omnibus/packagers/tarball.rb"

name 'ddot'
if fips_mode?
  package_name 'datadog-fips-agent-ddot'
else
  package_name 'datadog-agent-ddot'
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

# We want an higher compression level on deploy pipelines.
if ENV.has_key?("DEPLOY_AGENT") && ENV["DEPLOY_AGENT"] == "true"
  COMPRESSION_LEVEL = 9
else
  COMPRESSION_LEVEL = 5
end

if ENV.has_key?("OMNIBUS_GIT_CACHE_DIR")
  Omnibus::Config.use_git_caching true
  Omnibus::Config.git_cache_dir ENV["OMNIBUS_GIT_CACHE_DIR"]
end

if windows_target?
  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  INSTALL_DIR = 'C:/opt/datadog-agent'
else
  INSTALL_DIR = ENV["INSTALL_DIR"] || '/opt/datadog-agent'
end

install_dir INSTALL_DIR

third_party_licenses_path "LICENSES-ddot"
license_file_path "LICENSE-ddot"
json_manifest_path File.join(install_dir, "version-manifest.ddot.json")
text_manifest_path File.join(install_dir, "version-manifest.ddot.txt")

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Distribution of OpenTelemetry Collector'

# Determine whether this is build-only, package-only or end to end
do_build = false
do_package = false

if ENV["OMNIBUS_PACKAGE_ARTIFACT_DIR"]
  do_package = true
  skip_healthcheck true
else
  do_build = true
  if ENV["OMNIBUS_FORCE_PACKAGES"]
    do_package = true
  end
end

maintainer 'Datadog Packages <package@datadoghq.com>'

# ------------------------------------
# Dependencies
# ------------------------------------
if do_build
  dependency 'datadog-otel-agent'
elsif do_package
  dependency 'package-artifact'
  dependency 'init-scripts-ddot'
end

disable_version_manifest do_package
extra_package_file "/etc/datadog-agent/"

exclude '\.git*'
exclude 'bundler\/git'

# the stripper will drop the symbols in a `.debug` folder in the installdir
# we want to make sure that directory is not in the main build, while present
# in the debug package.
strip_build windows_target? || do_build
debug_path ".debug"  # the strip symbols will be in here

if windows_target?
  windows_symbol_stripping_file "#{install_dir}\\embedded\\bin\\otel-agent.exe"
  sign_file "#{install_dir}\\embedded\\bin\\otel-agent.exe"
end

# ------------------------------------
# Packaging
# ------------------------------------

# Maintainer names are chosen to match those on agent.rb
if debian_target?
  maintainer 'Datadog Packages <package@datadoghq.com>'
  # Use sanitized version to ensure it matches the actual version used for datadog-agent
  safe_version = Omnibus::Packager::DEB::safe_version(build_version)
  runtime_dependency "datadog-agent (= 1:#{safe_version}-1)"
  runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.4.0)'
elsif redhat_target? || suse_target?
  maintainer 'Datadog, Inc <package@datadoghq.com>'
  # RPM packages can't have dashes in their version segment, so we use
  # the same sanitization function that gets applied for the Agent version
  safe_version = Omnibus::Packager::RPM::safe_version(build_version)
  runtime_dependency "datadog-agent = 1:#{safe_version}-1"
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

package :msi do
  skip_packager true
end

package :xz do
  skip_packager do_package || (ENV["SKIP_PKG_COMPRESSION"] == "true")
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
end

# Uncompressed tar for faster local builds (skip the slow XZ compression)
package :tarball do
  skip_packager !(ENV["SKIP_PKG_COMPRESSION"] == "true")
end

# all flavors use the same package scripts
if linux_target?
  if do_build && !do_package
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/ddot-deb"
    extra_package_file "#{Omnibus::Config.project_root}/package-scripts/ddot-rpm"
  end
  if do_package
    if debian_target?
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/ddot-deb"
    else
      package_scripts_path "#{Omnibus::Config.project_root}/package-scripts/ddot-rpm"
    end
  end
end
