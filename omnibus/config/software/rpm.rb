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
default_version "4.20.0"

license "LGPLv2"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "config_guess"
dependency "file"
dependency "libgpg-error"
dependency "libgcrypt"
dependency "popt"
dependency "zstd"
dependency "libsqlite3"
dependency "libdb"
dependency "lua"

ship_source_offer true

version "4.20.0" do
  source url: "http://ftp.rpm.org/releases/rpm-4.20.x/rpm-#{version}.tar.bz2",
         sha256: "56ff7638cff98b56d4a7503ff59bc79f281a6ddffcda0d238c082bedfb5fbe7b"
end

relative_path "rpm-#{version}"

build do
  cmake_build_dir = "#{project_dir}/build"
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  patch source: "rpmdb-no-create.patch", env: env # don't create db if it doesn't exist already

  cmake_options = [
    "-DENABLE_NLS=OFF",
    "-DENABLE_OPENMP=OFF",
    "-DENABLE_PLUGINS=OFF",
    "-DWITH_ARCHIVE=OFF",
    "-DWITH_SELINUX=OFF",
    "-DWITH_IMAEVM=OFF",
    "-DWITH_CAP=OFF",
    "-DWITH_ACL=OFF",
    "-DWITH_AUDIT=OFF",
    "-DWITH_READLINE=OFF",
    "-DWITH_OPENSSL=ON",
    "-DWITH_LIBELF=OFF",
    "-DWITH_LIBDW=OFF",
    "-DWITH_SEQUOIA=OFF",
    "-DENABLE_TESTSUITE=OFF",
  ]

  cmake(*cmake_options, env: env, cwd: cmake_build_dir, prefix: "#{install_dir}/embedded")
end
