# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-agent-data-plane"

# We manually pull in SBOM/license files from the ADP tarball and place them in the appropriate location.
skip_transitive_dependency_licensing true

ADP_DEFAULT_VERSION = "1.3.0"
ADP_DEFAULT_HASHES = {
  "linux-amd64"        => "52b98149e3a5877309ba877332a3a9bd9b360cca2500de8bb406428491249470",
  "linux-arm64"        => "b552daa11401a2eef6e0b7b081b8a8daa115af98194049e9a427c305bb6a61a7",
  "fips-linux-amd64"   => "e74092b24c0bfc3d5b46ae919d20405ad6ab9acdb26efcafec7e40d2ba32cb0d",
  "fips-linux-arm64"   => "feb7dcbf13c53a7018171773c0750ddbe394ea4a21e0bf93e55b391e2a0fbc71",
  "darwin-amd64"       => "c46f14cb5818a570e61489b33b9c5e74d00b646fe4ab5f35b6f283499859c17b",
  "darwin-arm64"       => "e1ceb847f924501c5310aa24c28ff5b1a2f732a987113d2c082c1c50548cca30",
  "windows-amd64"      => "a1c1a148f75c76a418054dd52a78483d2b8e42f1bf91bc81b8b2aef3f6fbd69d",
  "fips-windows-amd64" => "f37b40496555b6dae391705a9a89a3807f297ff613a466138bd85ede7bcb0e52",
}.freeze

adp_version = ENV['AGENT_DATA_PLANE_VERSION'] || ADP_DEFAULT_VERSION
adp_hashes  = ADP_DEFAULT_HASHES.transform_values { |v| v }
ADP_DEFAULT_HASHES.each_key do |platform|
  env_key = "AGENT_DATA_PLANE_HASH_#{platform.upcase.tr('-', '_')}"
  adp_hashes[platform] = ENV[env_key] if ENV[env_key]
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
  raise "Missing Agent Data Plane hash for #{package_target}. Please update the hashes in datadog-agent-data-plane.rb."
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
