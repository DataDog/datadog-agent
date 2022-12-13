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
default_version "3.4.1"

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version '3.4.1' do
  source url: 'https://ftp.gnu.org/gnu/nettle/nettle-3.4.1.tar.gz',
         sha512: '26aefbbe9927e90e28f271e56d2ba876611831222d0e1e1a58bdb75bbd50934fcd84418a4fe47b845f557e60a9786a72a4de2676c930447b104f2256aca7a54f'
end

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "install_to_lib.patch", env: env

  env["CFLAGS"] << " -fPIC"
  configure "--disable-assembler --enable-mini-gmp", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
