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

name "libxxhash"
default_version "0.8.1"

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version("0.8.1") { source sha256: "3bb6b7d6f30c591dd65aaaff1c8b7a5b94d81687998ca9400082c739a690436c" }

source url: "https://github.com/Cyan4973/xxHash/archive/refs/tags/v0.8.1.tar.gz"

relative_path "xxHash-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
