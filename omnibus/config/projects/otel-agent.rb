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

unless linux_target?
    raise UnknownPlatform
end

INSTALL_DIR = ENV["INSTALL_DIR"] || '/opt/datadog-agent'

install_dir INSTALL_DIR

json_manifest_path File.join(install_dir, "version-manifest.otel-agent.json")
text_manifest_path File.join(install_dir, "version-manifest.otel-agent.txt")

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description ''

# ------------------------------------
# Generic package information
# ------------------------------------

package :deb do
    skip_packager true
end

package :xz do
    compression_threads COMPRESSION_THREADS
    compression_level COMPRESSION_LEVEL
end

# ------------------------------------
# Dependencies
# ------------------------------------
dependency 'datadog-otel-agent'

extra_package_file "#{output_config_dir}/etc/datadog-agent/"

exclude '\.git*'
exclude 'bundler\/git'

# the stripper will drop the symbols in a `.debug` folder in the installdir
# we want to make sure that directory is not in the main build, while present
# in the debug package.
strip_build true
debug_path ".debug"  # the strip symbols will be in here
