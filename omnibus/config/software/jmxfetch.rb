# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

name "jmxfetch"

default_version "0.18.1"

version "0.17.0" do
  source sha256: "e4bea1b045a3770736fbc1bc41cb37ebfd3b628d2180985e363b4b9cd8e77f95"
end

version "0.18.0" do
  source sha256: "a99edc3e2e82f2c08554ba310960e269e534f149b2cb17fd99dc3bfaec891190"
end

version "0.18.1" do
  source sha256: "7101da32c9d3fb0bd92cec735dea78f3614a40ce8c4a1f09877f3d6ef6c6f8f9"
end

source :url => "https://dd-jmxfetch.s3.amazonaws.com/jmxfetch-#{version}-jar-with-dependencies.jar"

jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/jmxfetch/master/LICENSE"
  mkdir jar_dir
  copy "jmxfetch-*-jar-with-dependencies.jar", jar_dir
end
