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

name "gmp"
default_version "6.2.1"

dependency 'nettle'

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version("6.2.1") { source sha256: "fd4829912cddd12f84181c3451cc752be224643e87fac497b69edddadc49b4f2" }

source url: "https://gmplib.org/download/gmp/gmp-6.2.1.tar.xz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  configure "", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
