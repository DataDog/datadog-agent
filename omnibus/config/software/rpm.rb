#
# Copyright 2012-2014 Chef Software, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

name "rpm"
default_version "4.18.1"

license "LGPLv2"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "config_guess"
dependency "elfutils"
dependency "file"
dependency "libgpg-error"
dependency "libgcrypt"
dependency "popt"
dependency "zstd"
dependency "sqlite"
dependency "libdb"
dependency "lua"

ship_source_offer true

version "4.18.1" do
  source url: "http://ftp.rpm.org/releases/rpm-4.18.x/rpm-#{version}.tar.bz2",
         sha256: "37f3b42c0966941e2ad3f10fde3639824a6591d07197ba8fd0869ca0779e1f56"
end

relative_path "rpm-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CC"] = "/usr/bin/gcc"
  env["CXX"] = "/usr/bin/g++"
  env["CFLAGS"] << " -fPIC"

  patch source: "0001-Include-fcntl.patch", env: env # fix build
  patch source: "rpmdb-no-create.patch", env: env # don't create db if it doesn't exist already

  # Build fixes since 4.18.1.
  patch source: "0417-Fix-compiler-error-on-clang.patch", env: env
  patch source: "0418-Move-variable-to-nearest-available-scope.patch", env: env

  update_config_guess
  
  env["SQLITE_CFLAGS"] ="-I#{install_dir}/embedded/include"
  env["SQLITE_LIBS"] ="-L#{install_dir}/embedded/lib -lsqlite3"
  env["LUA_CFLAGS"] ="-I#{install_dir}/embedded/include"
  env["LUA_LIBS"] ="-L#{install_dir}/embedded/lib -l:liblua.a -lm"

  configure_options = [
    "--enable-sqlite=yes",
    "--enable-bdb-ro=yes",
    "--disable-nls",
    "--disable-openmp",
    "--disable-plugins",
    "--without-archive",
    "--without-selinux",
    "--without-imaevm",
    "--without-cap",
    "--without-acl",
    "--without-audit",
    "--without-readline",
    "--with-crypto=openssl",
    "--localstatedir=/var", # use /var/lib/rpm database from the system
    "--disable-static",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
