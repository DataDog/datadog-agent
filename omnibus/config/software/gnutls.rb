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
default_version "3.6.16"

dependency 'nettle'
dependency 'gmp'
dependency 'libtasn1'

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version("3.6.16") { source sha256: "1b79b381ac283d8b054368b335c408fedcb9b7144e0c07f531e3537d4328f3b3" }

source url: "https://www.gnupg.org/ftp/gcrypt/gnutls/v3.6/gnutls-3.6.16.tar.xz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  configure "--disable-non-suiteb-curves --with-included-unistring --without-p11-kit --disable-hardware-acceleration", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
