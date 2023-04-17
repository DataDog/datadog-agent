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

name "m4"
default_version "1.4.19"

license "GPL-3.0"
license_file "COPYING"
skip_transitive_dependency_licensing true

version("1.4.19") { source sha256: "3be4a26d825ffdfda52a56fc43246456989a3630093cced3fbddf4771ee58a70" }

ship_source_offer true

source url: "https://ftp.gnu.org/gnu/m4/m4-#{version}.tar.gz"

relative_path "m4-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  command "./configure --prefix=#{install_dir}/embedded", env: env

  make "-j #{workers}", env: env
  make "-j #{workers} install", env: env
end
