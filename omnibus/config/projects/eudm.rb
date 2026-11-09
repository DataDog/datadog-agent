# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"
require "./lib/project_helpers.rb"

# The "eudm" (End User Device Monitoring) fleet installer extension layer for the datadog-agent
# OCI package (Windows only). It currently carries the AI Usage native host feature; this project
# builds + signs the native host binary and produces datadog-agent-eudm-<version>-1-x86_64.zip,
# which the agent_win_oci job passes to `datadog-package create --extension eudm=<dir>`.
# Mirrors omnibus/config/projects/ddot.rb.

name 'eudm'
package_name 'datadog-agent-eudm'

license "Apache-2.0"
license_file "../LICENSE"

third_party_licenses "../LICENSE-3rdparty.csv"

homepage 'http://www.datadoghq.com'

if ENV.has_key?("OMNIBUS_WORKERS_OVERRIDE")
  COMPRESSION_THREADS = ENV["OMNIBUS_WORKERS_OVERRIDE"].to_i
else
  COMPRESSION_THREADS = 1
end

if ENV.has_key?("OMNIBUS_GIT_CACHE_DIR")
  Omnibus::Config.use_git_caching true
  Omnibus::Config.git_cache_dir ENV["OMNIBUS_GIT_CACHE_DIR"]
end

INSTALL_DIR = ENV['INSTALL_DIR'] || raise('INSTALL_DIR must be set in tasks/omnibus.py')
install_dir INSTALL_DIR

third_party_licenses_path "LICENSES-eudm"
license_file_path "LICENSE-eudm"
json_manifest_path File.join(install_dir, "version-manifest.eudm.json")
text_manifest_path File.join(install_dir, "version-manifest.eudm.txt")

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Datadog AI Usage native messaging host'

maintainer 'Datadog Packages <package@datadoghq.com>'

# ------------------------------------
# Dependencies
# ------------------------------------
dependency 'ai-usage-agent'

# the stripper will drop the symbols in a `.debug` folder in the installdir
# we want to make sure that directory is not in the main build, while present
# in the debug package.
strip_build true
debug_path ".debug"  # the strip symbols will be in here

if windows_target?
  windows_symbol_stripping_file "#{install_dir}\\ai-usage-agent-native-host.exe"
  sign_file "#{install_dir}\\ai-usage-agent-native-host.exe"
end

# ------------------------------------
# Packaging
# ------------------------------------

# Windows only: skip the MSI packager and rely on the default Windows ZIP packager, which emits
# datadog-agent-eudm-<version>-1-x86_64.zip from the contents of install_dir.
package :msi do
  skip_packager true
end
