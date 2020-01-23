# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

name "jmxfetch"

jmxfetch_version = ENV['JMXFETCH_VERSION']
jmxfetch_hash = ENV['JMXFETCH_HASH']

if jmxfetch_version.nil? || jmxfetch_version.empty? || jmxfetch_hash.nil? || jmxfetch_hash.empty?
  raise "Please specify JMXFETCH_VERSION and JMXFETCH_HASH env vars to build."
end

default_version jmxfetch_version
source sha256: jmxfetch_hash

source url: "https://dl.bintray.com/datadog/datadog-maven/com/datadoghq/jmxfetch/#{version}/jmxfetch-#{version}-jar-with-dependencies.jar",
       target_filename: "jmxfetch.jar"


jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/jmxfetch/master/LICENSE"
  mkdir jar_dir

  if osx?
    # Also sign binaries and libraries inside the .jar, because they're detected by the Apple notarization service.
    command "unzip jmxfetch.jar -d ."
    delete "jmxfetch.jar"
    command "find . -type f | grep -E '(\\.so|\\.dylib|\\.jnilib)' | xargs codesign -o runtime --force --timestamp --deep -s 'Developer ID Application: Datadog, Inc. (JKFCB4CN7C)'"
    command "zip jmxfetch.jar -r ."
  end

  copy "jmxfetch.jar", "#{jar_dir}/jmxfetch.jar"
  block { File.chmod(0644, "#{jar_dir}/jmxfetch.jar") }
end
