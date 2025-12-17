# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-agent-data-plane"

# We manually pull in SBOM/license files from the ADP tarball and place them in the appropriate location.
skip_transitive_dependency_licensing true

adp_version = ENV['AGENT_DATA_PLANE_VERSION']
adp_hashes = {}
adp_hashes["linux-amd64"] = ENV['AGENT_DATA_PLANE_HASH_LINUX_AMD64']
adp_hashes["linux-arm64"] = ENV['AGENT_DATA_PLANE_HASH_LINUX_ARM64']
adp_hashes["fips-linux-amd64"] = ENV['AGENT_DATA_PLANE_HASH_FIPS_LINUX_AMD64']
adp_hashes["fips-linux-arm64"] = ENV['AGENT_DATA_PLANE_HASH_FIPS_LINUX_ARM64']

if adp_version.nil? || adp_version.empty? || adp_hashes.empty? || adp_hashes.any? { |k, v| v.nil? || v.empty? }
  raise "Please specify AGENT_DATA_PLANE_VERSION, AGENT_DATA_PLANE_HASH_LINUX_AMD64, AGENT_DATA_PLANE_HASH_LINUX_ARM64, AGENT_DATA_PLANE_HASH_FIPS_LINUX_AMD64, and AGENT_DATA_PLANE_HASH_FIPS_LINUX_ARM64 env vars to build."
end

default_version adp_version

# We don't want to build any dependencies in "repackaging mode" so all usual dependencies
# need to go under this guard.
unless do_repackage?
  # creates required build directories
  dependency 'datadog-agent-prepare'
end

if !linux_target?
  raise "Agent Data Plane is only built for Linux platforms currently and should not be included in non-Linux builds."
end

# Dynamically build the source URL/SHA256 hash based on the platform/architecture we're building for.
source_url_base = "https://binaries.ddbuild.io/saluki/"
base_package_name = "agent-data-plane-#{adp_version}"

target_arch = "amd64"
if arm_target?
  target_arch = "arm64"
end

package_target = "linux-#{target_arch}"

if fips_mode?
  package_target = "fips-#{package_target}"
end

source sha256: adp_hashes[package_target]
source url: "#{source_url_base}/#{base_package_name}-#{package_target}.tar.gz"

build do
  license :project_license

  copy 'opt/datadog-agent/embedded/bin/agent-data-plane', "#{install_dir}/embedded/bin"
  copy 'opt/datadog/agent-data-plane/LICENSES', "#{install_dir}/LICENSES"
  copy 'opt/datadog/agent-data-plane/LICENSE-3rdparty.csv', "#{install_dir}/LICENSES/LICENSE-agent-data-plane-3rdparty.csv"
end
