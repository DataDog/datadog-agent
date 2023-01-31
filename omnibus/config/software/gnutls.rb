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
default_version "3.7.8"

dependency 'nettle'
dependency 'gmp'
dependency 'libtasn1'

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version "3.7.8" do
  source url: "https://www.gnupg.org/ftp/gcrypt/gnutls/v3.7/gnutls-#{version}.tar.xz",
         sha256: "c58ad39af0670efe6a8aee5e3a8b2331a1200418b64b7c51977fb396d4617114"
end

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  env["CFLAGS"] << " -fPIC"

  configure "--disable-non-suiteb-curves --with-included-unistring --without-p11-kit --disable-hardware-acceleration --disable-static", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
