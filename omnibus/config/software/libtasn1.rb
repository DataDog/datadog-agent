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

name "libtasn1"
default_version "4.19.0"

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version("4.19.0") { source sha256: "1613f0ac1cf484d6ec0ce3b8c06d56263cc7242f1c23b30d82d23de345a63f7a" }

source url: "https://ftp.gnu.org/gnu/libtasn1/libtasn1-4.19.0.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  configure "--disable-static", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
