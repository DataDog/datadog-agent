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

name "pcre"
default_version "8.45"

license "BSD-2-Clause"
license_file "LICENCE"
skip_transitive_dependency_licensing true

dependency "libedit"
dependency "ncurses"
dependency "config_guess"

version("8.45") { source sha256: "4e6ce03e0336e8b4a3d6c2b70b1c5e18590a5673a98186da90d4f33c23defc09" }

source url: "http://downloads.sourceforge.net/project/pcre/pcre/#{version}/pcre-#{version}.tar.gz"

relative_path "pcre-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  update_config_guess

  command "./configure" \
          " --prefix=#{install_dir}/embedded" \
          " --disable-cpp" \
          " --enable-utf" \
          " --enable-unicode-properties" \
          " --enable-pcretest-libedit" \
          "--disable-pcregrep-jit", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
