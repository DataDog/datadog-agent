#
# Copyright:: Chef Software, Inc.
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

name "systemd"
default_version "252"

dependency 'nettle'
dependency 'gmp'
dependency 'libtasn1'

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version("252") { source sha256: "113a9342ddf89618a17c4056c2dd72c4b20b28af8da135786d7e9b4f1d18acfb" }

source url: "https://github.com/systemd/systemd/archive/refs/tags/v252.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  configure "", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
