# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

name "jmxfetch"

jmxfetch_version = ENV['JMXFETCH_VERSION']
jmxfetch_hash = ENV['JMXFETCH_HASH']

if jmxfetch_version.nil? || jmxfetch_version.empty?
  jmxfetch_version = '0.18.2'
  jmxfetch_hash = "ffbfc54929cbfc8e66214e01c1a0dd2509b73cdca500a72632718726215d5917"
end

default_version jmxfetch_version
source sha256: jmxfetch_hash

source :url => "https://dd-jmxfetch.s3.amazonaws.com/jmxfetch-#{version}-jar-with-dependencies.jar"

jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/jmxfetch/master/LICENSE"
  mkdir jar_dir
  copy "jmxfetch-#{jmxfetch_version}-jar-with-dependencies.jar", jar_dir
  block { File.chmod(0644, "#{jar_dir}/jmxfetch-#{jmxfetch_version}-jar-with-dependencies.jar") }
end
