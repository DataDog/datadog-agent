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

name "gnutls"
default_version "3.7.10"

dependency 'nettle'
dependency 'gmp'
dependency 'libtasn1'

license "LGPL-2.1"
license_file "doc/COPYING.LESSER"
skip_transitive_dependency_licensing true

version("3.7.10") { source sha256: "b6e4e8bac3a950a3a1b7bdb0904979d4ab420a81e74de8636dd50b467d36f5a9" }

ship_source_offer true

source url: "https://www.gnupg.org/ftp/gcrypt/gnutls/v3.7/gnutls-#{version}.tar.xz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  configure "--disable-non-suiteb-curves --with-included-unistring --without-p11-kit --disable-hardware-acceleration --disable-static", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
