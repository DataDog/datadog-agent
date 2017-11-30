# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.
#
require './lib/ostools.rb'

name "protobuf-py-cpp"
default_version "3.5.1"

dependency "python"
dependency "setuptools"
dependency "pip"
dependency "six"

source :url => "https://github.com/google/protobuf/releases/download/v#{version}/protobuf-python-#{version}.tar.gz",
       :sha256 => "13d3c15ebfad8c28bee203dd4a0f6e600d2a7d2243bac8b5d0e517466500fcae"

relative_path "protobuf-#{version}/python"

env = {
  "CFLAGS" => "-fPIC",
  "CXXFLAGS" => "-fPIC",
}

if ohai["platform_family"] == "mac_os_x"
  env["MACOSX_DEPLOYMENT_TARGET"] = "10.9"
end

build do
  ship_license "https://raw.githubusercontent.com/google/protobuf/3.5.x/LICENSE"

  if windows?
    pip "install protobuf==#{version}"
  else
    # C++ runtime
    command ["cd .. && ./configure",
                "--prefix=#{install_dir}/embedded",
                "--without-zlib"].join(" "), :env => env

    # You might want to temporarily uncomment the following line to check build sanity (e.g. when upgrading the
    # library) but there's no need to perform the check every time.
    # command "cd .. && make check"
    command "cd .. && make -j #{workers}"

    command ["#{install_dir}/embedded/bin/python",
             "setup.py",
             "build",
             "--cpp_implementation",
             "--compile_static_extension"].join(" "), :env => env
    command "#{install_dir}/embedded/bin/python setup.py test --cpp_implementation", :env => env
    pip "install . --install-option=\"--cpp_implementation\""
  end
end
