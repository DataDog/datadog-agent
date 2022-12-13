# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'openscap'
default_version '1.2.17'

version("1.3.6") { source sha256: "5e4d6c4addc15b2a0245b5caef80fda3020f1cac83ed4aa436ef3f1703d1d761060c931c2536fa68de7ad5bab002b79c8b2d1e5f7695d46249f4562f5a1569a0" }
version("1.2.17") { source sha256: "8a8cea880193b092895e1094dcc1368f8f44d986cf0749166e5da40ab6214982" }

source url: "https://github.com/OpenSCAP/openscap/releases/download/#{version}/openscap-#{version}.tar.gz"

dependency 'pcre'
dependency 'xmlsec'
dependency 'popt'
dependency 'curl'
dependency 'pcre'
dependency 'libxslt'
dependency 'libyaml'
dependency 'libgcrypt'
dependency 'bzip2'
dependency 'rpm'
dependency 'libacl'
dependency 'attr'
dependency 'libselinux'
dependency 'libsepol'
dependency 'apt'
dependency 'libdb'

relative_path "openscap-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "fix_apt_key_pkgconfig.patch", env: env
  patch source: "add_missing_apt_includes.patch", env: env
  patch source: "get_results_from_session.patch", env: env
  patch source: "5e5bc61c1fc6a6556665aa5689a62d6bc6487c74.patch", env: env

  env["CC"] = "/opt/datadog-agent/embedded/bin/gcc"
  env["CXX"] = "/opt/datadog-agent/embedded/bin/g++"

  #cmake_options = [
  #  "-D", "ENABLE_PYTHON3=FALSE",
  #  "-D", "ENABLE_PERL=FALSE",
  #]
  #cmake(*cmake_options, env: env)

  configure_options = []
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
