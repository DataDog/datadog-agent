# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'apt'
default_version '2.5.4'

dependency 'gcc'
dependency 'gnutls'
dependency 'liblz4'
dependency 'liblzma'
dependency 'libxxhash'

version '2.5.4' do
  source url: 'https://github.com/Debian/apt/archive/refs/tags/2.5.4.tar.gz',
         sha512: '41c27e14c1a817ea578d88db50f1223a7add30c990fc9befb81aef27f735d75f53009cd914b11fca0a5c16deb9dc40c86fc91fb05b66c83e5a3ba9f647e3531c'
end

version '1.9.0' do
  source url: 'https://github.com/Debian/apt/archive/refs/tags/1.9.0.tar.gz',
         sha512: '4644a47a416d58f4e049c0e2e91a72dbe85f66c130d03babdafa052aab344d927a86306688719be6fcd33eb17df9f6d0b25132348fda1c8af28a4d470fa860d9'
end

version '1.7.0' do
  source url: 'https://github.com/Debian/apt/archive/refs/tags/1.7.0.tar.gz',
         sha512: 'ac4279256bef243294053e97e5a505523f4eca51bdd1d2a92832f308f4fa863770cf2a2c07d160225de6deae3c955cb8c1bb97159a3883993759c99e7b9e1046'
end

version '1.2.35' do
  source url: 'https://launchpad.net/ubuntu/+archive/primary/+sourcefiles/apt/1.2.35/apt_1.2.35.tar.xz',
         sha512: '4d34d4f386eadcdc6bb16befbd752d18c0a8e62b69f3e49659ee051eb58579477b2c708df2507f917294e650384b63a37ac6432b82d93205ce0f26f6b772c32f'
end

relative_path "apt-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["PKG_CONFIG_PATH"] = "/opt/datadog-agent/embedded/lib/pkgconfig"
  env["CC"] = "/opt/datadog-agent/embedded/bin/gcc"
  env["CXX"] = "/opt/datadog-agent/embedded/bin/g++"
  env["CXXFLAGS"] += " -std=c++11 -DDPKG_DATADIR=/usr/share/dpkg"
  env["CPPFLAGS"] += " -std=c++11 -DDPKG_DATADIR=/usr/share/dpkg"
  patch source: "no_doc.patch", env: env
  patch source: "export_mmap.patch", env: env
  # patch source: "hardcode_paths.patch", env: env
  patch source: "log_opens.patch", env: env
  patch source: "debug.patch", env: env
  patch source: "disable_arch_check.patch", env: env
  patch source: "do_not_clean_pkgcache.patch", env: env

  cmake_options = [
    "-DDPKG_DATADIR=/usr/share/dpkg",
    "-DCMAKE_INSTALL_FULL_SYSCONFDIR:PATH=/etc",
    "-DCONF_DIR:PATH=/etc/apt",
    "-DCACHE_DIR:PATH=/opt/datadog-agent/run",
    # "-DCACHE_DIR=/opt/datadog-agent/run",
    "-DSTATE_DIR:PATH=/var/lib/apt",
    "-DWITH_DOC=OFF",
    "-DUSE_NLS=OFF",
    "-DWITH_TESTS=OFF",
    "-DLZMA_LIBRARIES:FILEPATH=/opt/datadog-agent/embedded/lib/liblzma.so",
  ]
  cmake(*cmake_options, env: env)

  #update_config_guess

  configure_options = [
  ]
  #configure(*configure_options, env: env)

end
