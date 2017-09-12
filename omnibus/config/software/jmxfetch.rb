# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

name "jmxfetch"

default_version "0.16.0"
source :url => "https://dd-jmxfetch.s3.amazonaws.com/jmxfetch-#{version}-jar-with-dependencies.jar",
       :sha256 => "7ca7aee7ba63e5938df35bb6327d7b10c86ed800a88e6c8173a4f5931a25641d"

jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/jmxfetch/master/LICENSE"
  mkdir jar_dir
  copy "jmxfetch-*-jar-with-dependencies.jar", jar_dir
end