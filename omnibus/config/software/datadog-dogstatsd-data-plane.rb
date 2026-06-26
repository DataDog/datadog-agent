# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Bundles the Agent Data Plane (ADP) binary into the standalone dogstatsd
# package so it can optionally be run as the metrics pipeline alongside (or in
# place of) the Go dogstatsd process. This mirrors `datadog-agent-data-plane`
# but installs into the dogstatsd install tree (/opt/datadog-dogstatsd).

name "datadog-dogstatsd-data-plane"

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

if adp_version.nil? || adp_version.empty?
  raise "Please specify AGENT_DATA_PLANE_VERSION to build Agent Data Plane."
end

default_version adp_version

# Dynamically build the source URL/SHA256 hash based on the platform/architecture we're building for.
source_url_base = ENV['AGENT_DATA_PLANE_SOURCE_URL_BASE']
source_url_base = 'https://binaries.ddbuild.io/saluki' if source_url_base.nil? || source_url_base.empty?
source_url_base = source_url_base.sub(%r{/*$}, '')
base_package_name = "agent-data-plane-#{adp_version}"

target_arch = arm_target? ? "arm64" : "amd64"

if linux_target?
  package_target = "linux-#{target_arch}"
  package_target = "fips-#{package_target}" if fips_mode?
elsif osx_target?
  if fips_mode?
    raise "Agent Data Plane FIPS artifacts are not available for macOS."
  end
  package_target = "darwin-#{target_arch}"
else
  raise "Agent Data Plane is only packaged for Linux and macOS."
end

adp_hash = adp_hashes[package_target]
if adp_hash.nil? || adp_hash.empty?
  raise "Please specify AGENT_DATA_PLANE_HASH_#{package_target.upcase.tr('-', '_')} to build Agent Data Plane #{package_target}."
end

source sha256: adp_hash
source url: "#{source_url_base}/#{base_package_name}-#{package_target}.tar.gz"

build do
  license :project_license

  mkdir "#{install_dir}/embedded/bin"

  copy 'opt/datadog-agent/embedded/bin/agent-data-plane', "#{install_dir}/embedded/bin"
  command "chmod 0755 #{install_dir}/embedded/bin/agent-data-plane"
  copy 'opt/datadog/agent-data-plane/LICENSES', "#{install_dir}/LICENSES"
  copy 'opt/datadog/agent-data-plane/LICENSE-3rdparty.csv', "#{install_dir}/LICENSES/LICENSE-agent-data-plane-3rdparty.csv"

  # Service files used to run the data plane alongside dogstatsd. They are
  # rendered into scripts/ here and moved to their final system locations by
  # datadog-dogstatsd-finalize.
  if linux_target?
    if debian_target?
      erb source: "upstart_debian.conf.erb",
          dest: "#{install_dir}/scripts/datadog-dogstatsd-data-plane.conf",
          mode: 0644,
          vars: { install_dir: install_dir }
    elsif redhat_target? || suse_target?
      erb source: "upstart_redhat.conf.erb",
          dest: "#{install_dir}/scripts/datadog-dogstatsd-data-plane.conf",
          mode: 0644,
          vars: { install_dir: install_dir }
    end
    erb source: "systemd.service.erb",
        dest: "#{install_dir}/scripts/datadog-dogstatsd-data-plane.service",
        mode: 0644,
        vars: { install_dir: install_dir }
  end
end
