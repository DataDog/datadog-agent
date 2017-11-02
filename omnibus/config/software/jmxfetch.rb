# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

name "jmxfetch"

default_version "0.18.0"

version "0.17.0" do
  source sha256: "e4bea1b045a3770736fbc1bc41cb37ebfd3b628d2180985e363b4b9cd8e77f95"
end

version "0.18.0" do
  source sha256: "a99edc3e2e82f2c08554ba310960e269e534f149b2cb17fd99dc3bfaec891190"
end

source :url => "https://dd-jmxfetch.s3.amazonaws.com/jmxfetch-#{version}-jar-with-dependencies.jar"

jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/jmxfetch/master/LICENSE"
  mkdir jar_dir
  copy "jmxfetch-*-jar-with-dependencies.jar", jar_dir
end
