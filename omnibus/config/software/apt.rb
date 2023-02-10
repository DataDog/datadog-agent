# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'apt'
default_version '2.5.5'

dependency 'bzip2'
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

  env["PKG_CONFIG_PATH"] = "#{install_dir}/embedded/lib/pkgconfig"
  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"
  env["CXX"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/g++"
  env["CXXFLAGS"] += " -static-libstdc++ -std=c++11"
  patch source: "no_doc.patch", env: env
  patch source: "disable_arch_check.patch", env: env
  patch source: "disable_systemd.patch", env: env
  patch source: "cmake-bindir.patch", env: env

  if (!File.exist? '/usr/bin/triehash') && (!File.exist? '/usr/local/bin/triehash')
    patch source: "triehash.patch", env: env, cwd: '/'
    command "chmod +x /usr/local/bin/triehash"
  end

# Don't rely on dpkg.
if intel? && _64_bit?
  arch = 'amd64'
end
if arm? && _32_bit?
  arch = 'arm'
end
if arm? && _64_bit?
  arch = 'arm64'
end

  cmake_options = [
    "-DCURRENT_VENDOR=ubuntu",
    "-DCOMMON_ARCH=#{arch}",
    "-DDPKG_DATADIR=/usr/share/dpkg",
    "-DBUILD_STATIC_LIBS=OFF",
    "-DSTATE_DIR:PATH=/var/lib/apt",
    "-DCACHE_DIR:PATH=/var/cache/apt",
    "-DLOG_DIR:PATH=/var/log/apt",
    "-DCONF_DIR:PATH=/etc/apt",
    "-DLIBEXEC_DIR:PATH=/usr/lib/apt",
    "-DBIN_DIR:PATH=/usr/bin", # use dpkg from the system
    "-DWITH_DOC=OFF",
    "-DUSE_NLS=OFF",
    "-DWITH_TESTS=OFF",
    "-DLZMA_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/liblzma.so",
    "-DGCRYPT_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/libgcrypt.so",
  ]
  cmake(*cmake_options, env: env)

end
