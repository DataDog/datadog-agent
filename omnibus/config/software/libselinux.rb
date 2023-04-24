#
# Copyright 2012-2019, Chef Software Inc.
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

name "libselinux"
default_version "3.4"

dependency 'pcre2'
dependency 'libsepol'

license "Public-Domain"
license_file "LICENSE"
skip_transitive_dependency_licensing true

version("3.4") { source sha256: "77c294a927e6795c2e98f74b5c3adde9c8839690e9255b767c5fca6acff9b779" }

source url: "https://github.com/SELinuxProject/selinux/releases/download/#{version}/libselinux-#{version}.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "ln_no_relative.patch", env: env # fix build with old glibc
  patch source: "fcntl_o_path.patch", env: env # don't use relative symlink on installed libraries

  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "-j #{workers} install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
