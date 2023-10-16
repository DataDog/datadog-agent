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

name "liblz4"
default_version "1.9.4"

license "BSD-2-Clause"
license_file "lib/LICENSE"
skip_transitive_dependency_licensing true

version("1.9.4") { source sha256: "0b0e3aa07c8c063ddf40b082bdf7e37a1562bda40a0ff5272957f3e987e0e54b" }

source url: "https://github.com/lz4/lz4/archive/refs/tags/v#{version}.tar.gz"

relative_path "lz4-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  make "-C lib/ -j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded BUILD_STATIC=no", env: env
  make "-C lib/ install PREFIX=/ DESTDIR=#{install_dir}/embedded BUILD_STATIC=no", env: env
end
