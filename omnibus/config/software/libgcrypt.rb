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

name "libgcrypt"
default_version "1.10.2"

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

dependency "libgpg-error"

version("1.10.2") { source sha256: "3b9c02a004b68c256add99701de00b383accccf37177e0d6c58289664cce0c03" }

ship_source_offer true

source url: "https://www.gnupg.org/ftp/gcrypt/libgcrypt/libgcrypt-#{version}.tar.bz2"

relative_path "libgcrypt-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  
  env["CFLAGS"] = "-I#{install_dir}/embedded/include -O1 -D_FORTIFY_SOURCE=1 -fPIC"

  configure_options = [
    "--prefix=#{install_dir}/embedded",
    "--enable-maintainer-mode",
  ]

  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
