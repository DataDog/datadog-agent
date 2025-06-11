# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"
require "./lib/project_helpers.rb"
output_config_dir = ENV["OUTPUT_CONFIG_DIR"]

name 'otel-agent'
package_name 'datadog-otel-agent'

license "Apache-2.0"
license_file "../LICENSE"

third_party_licenses "../LICENSE-3rdparty.csv"

homepage 'http://www.datadoghq.com'

if ENV.has_key?("OMNIBUS_WORKERS_OVERRIDE")
  COMPRESSION_THREADS = ENV["OMNIBUS_WORKERS_OVERRIDE"].to_i
else
  COMPRESSION_THREADS = 1
end

# We want an higher compression level on deploy pipelines that are not nightly.
# Nightly pipelines will be used as main reference for static quality gates and need the same compression level as main.
if ENV.has_key?("DEPLOY_AGENT") && ENV["DEPLOY_AGENT"] == "true" && ENV.has_key?("BUCKET_BRANCH") && ENV['BUCKET_BRANCH'] != "nightly"
  COMPRESSION_LEVEL = 9
else
  COMPRESSION_LEVEL = 5
end

if ENV.has_key?("OMNIBUS_GIT_CACHE_DIR")
  Omnibus::Config.use_git_caching true
  Omnibus::Config.git_cache_dir ENV["OMNIBUS_GIT_CACHE_DIR"]
end

INSTALL_DIR = ENV["INSTALL_DIR"] || '/opt/datadog-agent'

install_dir INSTALL_DIR

third_party_licenses_path "LICENSES-ddot"
license_file_path "LICENSE-ddot"
json_manifest_path File.join(install_dir, "version-manifest.otel-agent.json")
text_manifest_path File.join(install_dir, "version-manifest.otel-agent.txt")

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog Distribution of OTel Collector'

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
end

disable_version_manifest do_package
extra_package_file "#{output_config_dir}/etc/datadog-agent/"

exclude '\.git*'
exclude 'bundler\/git'

# the stripper will drop the symbols in a `.debug` folder in the installdir
# we want to make sure that directory is not in the main build, while present
# in the debug package.
strip_build do_build
debug_path ".debug"  # the strip symbols will be in here

# ------------------------------------
# Packaging
# ------------------------------------

if debian_target?
  runtime_dependency "datadog-agent (= 1:#{build_version}-1)"
  runtime_recommended_dependency 'datadog-signing-keys (>= 1:1.4.0)'
elsif redhat_target?
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

package :xz do
  skip_packager do_package
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
end
