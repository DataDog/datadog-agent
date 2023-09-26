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
default_version "0.8.2"

license "BSD-2-Clause"
license_file "LICENSE"
skip_transitive_dependency_licensing true

version("0.8.2") { source sha256: "baee0c6afd4f03165de7a4e67988d16f0f2b257b51d0e3cb91909302a26a79c4" }

source url: "https://github.com/Cyan4973/xxHash/archive/refs/tags/v#{version}.tar.gz"

relative_path "xxHash-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
