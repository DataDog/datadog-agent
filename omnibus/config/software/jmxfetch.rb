# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "jmxfetch"

jmxfetch_version = ENV['JMXFETCH_VERSION']
jmxfetch_hash = ENV['JMXFETCH_HASH']

if jmxfetch_version.nil? || jmxfetch_version.empty? || jmxfetch_hash.nil? || jmxfetch_hash.empty?
  raise "Please specify JMXFETCH_VERSION and JMXFETCH_HASH env vars to build."
end

default_version jmxfetch_version

source sha256: jmxfetch_hash

if jmxfetch_snapshot_version = Regexp.new('(?<current_version>\d+\.\d+\.\d+)[-]').freeze.match(jmxfetch_version)
    license_file_version = 'master'
    jmxfetch_snapshot_version = jmxfetch_snapshot_version['current_version']
    source url: "https://oss.sonatype.org/content/repositories/snapshots/com/datadoghq/jmxfetch/#{jmxfetch_snapshot_version}-SNAPSHOT/jmxfetch-#{version}-jar-with-dependencies.jar",
           target_filename: "jmxfetch.jar"
else
    license_file_version = jmxfetch_version
    source url: "https://oss.sonatype.org/service/local/repositories/releases/content/com/datadoghq/jmxfetch/#{version}/jmxfetch-#{version}-jar-with-dependencies.jar",
           target_filename: "jmxfetch.jar"
end

jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  license "BSD-3-Clause"
  license_file "https://raw.githubusercontent.com/DataDog/jmxfetch/#{license_file_version}/LICENSE"

  mkdir jar_dir

  if osx_target? && code_signing_identity
    # Also sign binaries and libraries inside the .jar, because they're detected by the Apple notarization service.
    command "unzip jmxfetch.jar -d ."
    delete "jmxfetch.jar"

    if ENV['HARDENED_RUNTIME_MAC'] == 'true'
      hardened_runtime = "-o runtime --entitlements #{entitlements_file} "
    else
      hardened_runtime = ""
    end

    command "find . -type f | grep -E '(\\.so|\\.dylib|\\.jnilib)' | xargs -I{} codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '{}'"
    command "zip jmxfetch.jar -r ."
  end

  copy "jmxfetch.jar", "#{jar_dir}/jmxfetch.jar"
  block { File.chmod(0644, "#{jar_dir}/jmxfetch.jar") }
end
