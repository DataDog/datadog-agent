# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'openscap'
default_version '1.3.7'

version("1.3.7") { source sha256: "a74f5bfb420b748916d2f88941bb6e04cad4c67a4cafc78c96409cc15c54d1d3" }

source url: "https://github.com/OpenSCAP/openscap/releases/download/#{version}/openscap-#{version}.tar.gz"

dependency 'apt'
dependency 'attr'
dependency 'bzip2'
dependency 'curl'
dependency 'libacl'
dependency 'libgcrypt'
dependency 'libselinux'
dependency 'libsepol'
dependency 'libxslt'
dependency 'libyaml'
dependency 'pcre'
dependency 'popt'
dependency 'rpm'
dependency 'xmlsec'

relative_path "openscap-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "get_results_from_session.patch", env: env
  patch source: "010_perlpm_install_fix.patch", env: env
  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"
  env["CXX"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/g++"
  env["CXXFLAGS"] += " -static-libstdc++ -std=c++11 -DDPKG_DATADIR=/usr/share/dpkg"

  cmake_build_dir = "#{project_dir}/build"
  cmake_options = [
    "-DENABLE_PERL=OFF",
    "-DENABLE_PYTHON3=OFF",
    "-DGCRYPT_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DGCRYPT_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libgcrypt.so",
  ]
  cmake(*cmake_options, env: env, cwd: cmake_build_dir, prefix: "#{install_dir}/embedded")
end
