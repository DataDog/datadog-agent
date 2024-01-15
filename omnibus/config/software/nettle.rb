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

name "nettle"
default_version "3.9.1"

license "LGPL-3.0-or-later"
license_file "COPYING.LESSERv3"
skip_transitive_dependency_licensing true

version("3.9.1") { source sha256: "ccfeff981b0ca71bbd6fbcb054f407c60ffb644389a5be80d6716d5b550c6ce3" }

ship_source_offer true

source url: "https://ftp.gnu.org/gnu/nettle/nettle-#{version}.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "install_to_lib.patch", env: env # omit lib directory suffix

  env["CFLAGS"] << " -fPIC"
  configure "--disable-assembler --enable-mini-gmp --disable-static", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
