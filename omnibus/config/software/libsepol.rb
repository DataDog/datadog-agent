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
default_version "3.4"

license "LGPL-2.1"
license_file "COPYING"
skip_transitive_dependency_licensing true

version("3.4") { source sha256: "fc277ac5b52d59d2cd81eec8b1cccd450301d8b54d9dd48a993aea0577cf0336" }

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
