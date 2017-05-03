name "jmxfetch"

jmx_version = ENV["JMX_VERSION"]
if jmx_version.nil? || jmx_version.empty?
  default_version "0.13.0"
else
  default_version jmx_version
end

version "0.12.0" do
  source md5: "2a04e4f02de90b7bbd94e581afb73c8f"
end

version "0.13.0" do
  source sha256: "414f02f1358477dab80d10f226972614c50ddb8a068cadc58e7d0a824fb02e0b"
end

source :url => "https://dd-jmxfetch.s3.amazonaws.com/jmxfetch-#{version}-jar-with-dependencies.jar"

relative_path "jmxfetch"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/jmxfetch/master/LICENSE"
  jar_dir = "#{install_dir}/bin/agent/dist/jmx"
  mkdir jar_dir
  copy "jmxfetch-*-jar-with-dependencies.jar", jar_dir
end