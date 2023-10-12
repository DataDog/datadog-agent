# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'apt'
default_version '2.7.3'

dependency 'bzip2'
dependency 'gnutls'
dependency 'libdb'
dependency 'libgcrypt'
dependency 'libiconv'
dependency 'liblz4'
dependency 'liblzma'
dependency 'libxxhash'
dependency 'zstd'

license 'GPLv2'
license_file "COPYING"
skip_transitive_dependency_licensing true

version("2.7.3") { source sha256: "67bf0a8f167a4124f9e93d06e17e40a77cce3032b146f6084acf9cc99011fca7" }

ship_source_offer true

source url: "https://github.com/Debian/apt/archive/refs/tags/#{version}.tar.gz"

relative_path "apt-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["PKG_CONFIG_PATH"] = "#{install_dir}/embedded/lib/pkgconfig"
  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"
  env["CXX"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/g++"
  env["CXXFLAGS"] += " -static-libstdc++ -std=c++11"
  patch source: "no_doc.patch", env: env # don't install documentation
  patch source: "disable_arch_check.patch", env: env # disable architecture check, so we could use APT on a system with different architectures installed
  patch source: "disable_systemd.patch", env: env # remove dependency on udev and systemd
  patch source: "cmake-bindir.patch", env: env # allow to update BIN_DIR CMake CACHE entry

  # Add the triehash tool, required at build time.
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
    "-DBERKELEY_INCLUDE_DIRS:PATH=#{install_dir}/embedded/include",
    "-DBERKELEY_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/libdb.so",
    "-DBZIP2_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DBZIP2_LIBRARY_RELEASE:FILEPATH=#{install_dir}/embedded/lib/libbz2.so",
    "-DGCRYPT_INCLUDE_DIRS:PATH=#{install_dir}/embedded/include",
    "-DGCRYPT_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/libgcrypt.so",
    "-DGNUTLS_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DGNUTLS_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libgnutls.so",
    "-DICONV_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DICONV_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libiconv.so",
    "-DLZ4_INCLUDE_DIRS:PATH=#{install_dir}/embedded/include",
    "-DLZ4_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/liblz4.so",
    "-DLZMA_INCLUDE_DIRS:PATH=#{install_dir}/embedded/include",
    "-DLZMA_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/liblzma.so",
    "-DXXHASH_INCLUDE_DIRS:PATH=#{install_dir}/embedded/include",
    "-DXXHASH_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/libxxhash.so",
    "-DZLIB_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DZLIB_LIBRARY_RELEASE:FILEPATH=#{install_dir}/embedded/lib/libz.so",
    "-DZSTD_INCLUDE_DIRS:PATH=#{install_dir}/embedded/include",
    "-DZSTD_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/libzstd.so",
  ]
  cmake(*cmake_options, env: env)

end
