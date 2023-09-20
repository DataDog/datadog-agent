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

name "libsepol"
default_version "3.5"

license "LGPL-2.1"
license_file "COPYING"
skip_transitive_dependency_licensing true

version("3.5") { source sha256: "78fdaf69924db780bac78546e43d9c44074bad798c2c415d0b9bb96d065ee8a2" }

ship_source_offer true

source url: "https://github.com/SELinuxProject/selinux/releases/download/#{version}/libsepol-#{version}.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "ln_no_relative.patch", env: env # don't use relative symlink on installed libraries

  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "-j #{workers} install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
