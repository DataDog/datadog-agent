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
default_version "4.14.1"

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
dependency 'libdb'

version "4.16.0" do
  source url: "http://ftp.rpm.org/releases/rpm-4.16.x/rpm-4.16.0.tar.bz2",
         sha256: "ca5974e9da2939afb422598818ef187385061889ba766166c4a3829c5ef8d411"
end

version "4.14.1" do
  source url: "http://ftp.rpm.org/releases/rpm-4.14.x/rpm-4.14.1.tar.bz2",
         sha256: "43f40e2ccc3ca65bd3238f8c9f8399d4957be0878c2e83cba2746d2d0d96793b"
end

relative_path "rpm-#{version}"

build do
  # patch source: "0001-Include-fcntl.patch"
  patch source: "0002-Set-backend-db-to-sqlite-by-default-in-the-macros.patch"
  patch source: "disable_md2.patch"

  env = with_standard_compiler_flags(with_embedded_path)

  env["CC"] = "/usr/bin/gcc"
  env["CXX"] = "/usr/bin/g++"
  env["CFLAGS"] << " -fPIC"

  patch source: "0001-Include-fcntl.patch", env: env

  update_config_guess
  
  env["SQLITE_CFLAGS"] ="-I#{install_dir}/embedded/include"
  env["SQLITE_LIBS"] ="-L#{install_dir}/embedded/lib -lsqlite3"

  configure_options = [
    "--enable-sqlite=yes",
    "--enable-bdb=no",
    "--disable-nls",
    "--disable-openmp",
    "--disable-plugins",
    "--without-archive",
    "--without-selinux",
    "--without-imaevm",
    "--without-cap",
    "--without-acl",
    "--without-lua",
    "--without-audit",
    "--with-crypto=openssl",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
