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
adp_hashes["darwin-amd64"] = ENV['AGENT_DATA_PLANE_HASH_DARWIN_AMD64']
adp_hashes["darwin-arm64"] = ENV['AGENT_DATA_PLANE_HASH_DARWIN_ARM64']
adp_hashes["windows-amd64"] = ENV['AGENT_DATA_PLANE_HASH_WINDOWS_AMD64']
adp_hashes["fips-windows-amd64"] = ENV['AGENT_DATA_PLANE_HASH_FIPS_WINDOWS_AMD64']

if adp_version.nil? || adp_version.empty?
  raise "Please specify AGENT_DATA_PLANE_VERSION to build Agent Data Plane."
end

default_version adp_version

# We don't want to build any dependencies in "repackaging mode" so all usual dependencies
# need to go under this guard.
unless do_repackage?
  # creates required build directories
  dependency 'datadog-agent-prepare'
end

# Dynamically build the source URL/SHA256 hash based on the platform/architecture we're building for.
source_url_base = ENV['AGENT_DATA_PLANE_SOURCE_URL_BASE']
source_url_base = 'https://binaries.ddbuild.io/saluki' if source_url_base.nil? || source_url_base.empty?
source_url_base = source_url_base.sub(%r{/*$}, '')
base_package_name = "agent-data-plane-#{adp_version}"

target_arch = arm_target? ? "arm64" : "amd64"

package_extension = "tar.gz"

if linux_target?
  package_target = "linux-#{target_arch}"
  package_target = "fips-#{package_target}" if fips_mode?
elsif osx_target?
  if fips_mode?
    raise "Agent Data Plane FIPS artifacts are not available for macOS."
  end
  package_target = "darwin-#{target_arch}"
elsif windows_target?
  if target_arch != "amd64"
    raise "Agent Data Plane Windows artifacts are only available for amd64."
  end
  package_target = "windows-#{target_arch}"
  if fips_mode?
    # The saluki CI publishes the Windows FIPS zip with the fips suffix
    # (e.g. windows-amd64-fips.zip), unlike Linux which embeds fips in the
    # version string (e.g. fips-linux-amd64.tar.gz). Preserve the existing
    # release.json key name (AGENT_DATA_PLANE_HASH_FIPS_WINDOWS_AMD64 →
    # "fips-windows-amd64") while using the correct filename suffix for S3.
    adp_hash_key = "fips-#{package_target}"
    package_target = "#{package_target}-fips"
  end
  package_extension = "zip"
else
  raise "Agent Data Plane is only packaged for Linux, macOS, and Windows."
end

adp_hash_key ||= package_target
adp_hash = adp_hashes[adp_hash_key]
if adp_hash.nil? || adp_hash.empty?
  raise "Please specify AGENT_DATA_PLANE_HASH_#{adp_hash_key.upcase.tr('-', '_')} to build Agent Data Plane #{package_target}."
end

source sha256: adp_hash
source url: "#{source_url_base}/#{base_package_name}-#{package_target}.#{package_extension}"

build do
  license :project_license

  if windows_target?
    copy 'bin/agent-data-plane.exe', "#{install_dir}/bin/agent"
    copy 'LICENSES', "#{install_dir}/LICENSES"
    copy 'LICENSE-3rdparty.csv', "#{install_dir}/LICENSES/LICENSE-agent-data-plane-3rdparty.csv"
  else
    copy 'opt/datadog-agent/embedded/bin/agent-data-plane', "#{install_dir}/embedded/bin"
    command "chmod 0755 #{install_dir}/embedded/bin/agent-data-plane"
    copy 'opt/datadog/agent-data-plane/LICENSES', "#{install_dir}/LICENSES"
    copy 'opt/datadog/agent-data-plane/LICENSE-3rdparty.csv', "#{install_dir}/LICENSES/LICENSE-agent-data-plane-3rdparty.csv"
  end
end
