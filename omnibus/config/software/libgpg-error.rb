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

name "libgpg-error"
default_version "1.39"

license "LGPL-2.1"
license_file "COPYING.LIB"
skip_transitive_dependency_licensing true

version("1.39") { source sha256: "4a836edcae592094ef1c5a4834908f44986ab2b82e0824a0344b49df8cdb298f" }

source url: "https://www.gnupg.org/ftp/gcrypt/libgpg-error/libgpg-error-#{version}.tar.bz2"

relative_path "libgpg-error-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  configure_options = [
    "--prefix=#{install_dir}/embedded",
    "--enable-maintainer-mode",
  ]

  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
