# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'apt'
default_version '2.5.5'

dependency 'gnutls'
dependency 'liblz4'
dependency 'liblzma'
dependency 'libxxhash'

license 'GPLv2'
license_file "COPYING"
skip_transitive_dependency_licensing true

version("2.5.5") { source sha256: "488d858485bd87369338cdbe3dcd74437379eebea9c9adf272df1e4a05714f3c" }

source url: "https://github.com/Debian/apt/archive/refs/tags/#{version}.tar.gz"

relative_path "apt-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["PKG_CONFIG_PATH"] = "/opt/datadog-agent/embedded/lib/pkgconfig"
  env["CC"] = "/opt/gcc-10.4.0/bin/gcc"
  env["CXX"] = "/opt/gcc-10.4.0/bin/g++"
  env["CXXFLAGS"] += " -static-libstdc++ -std=c++11 -DDPKG_DATADIR=/usr/share/dpkg"
  patch source: "no_doc.patch", env: env
  patch source: "disable_arch_check.patch", env: env
  patch source: "disable_systemd.patch", env: env

  if (!File.exist? '/usr/bin/triehash') && (!File.exist? '/usr/local/bin/triehash')
    patch source: "triehash.patch", env: env, cwd: '/'
    command "chmod +x /usr/local/bin/triehash"
  end

  cmake_options = [
    "-DDPKG_DATADIR=/usr/share/dpkg",
    "-DCMAKE_INSTALL_FULL_SYSCONFDIR:PATH=/etc",
    "-DBUILD_STATIC_LIBS=OFF",
    "-DCONF_DIR:PATH=/etc/apt",
    "-DCACHE_DIR:PATH=/opt/datadog-agent/run",
    "-DSTATE_DIR:PATH=/var/lib/apt",
    "-DWITH_DOC=OFF",
    "-DUSE_NLS=OFF",
    "-DWITH_TESTS=OFF",
    "-DLZMA_LIBRARIES:FILEPATH=/opt/datadog-agent/embedded/lib/liblzma.so",
    "-DGCRYPT_LIBRARIES:FILEPATH=/opt/datadog-agent/embedded/lib/libgcrypt.so",
  ]
  cmake(*cmake_options, env: env)

end
