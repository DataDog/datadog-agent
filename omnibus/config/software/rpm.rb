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
default_version "4.18.0"

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

version "4.18.0" do
  source url: "http://ftp.rpm.org/releases/rpm-4.18.x/rpm-#{version}.tar.bz2",
         sha256: "2a17152d7187ab30edf2c2fb586463bdf6388de7b5837480955659e5e9054554"
end

relative_path "rpm-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CC"] = "/usr/bin/gcc"
  env["CXX"] = "/usr/bin/g++"
  env["CFLAGS"] << " -fPIC"

  patch source: "0001-Include-fcntl.patch", env: env

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
