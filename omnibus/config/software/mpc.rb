#
# Copyright 2014-2019 Chef Software, Inc.
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

name "mpc"
default_version "1.2.1"

dependency "gmp"
dependency "mpfr"

license "LGPL-3.0-or-later"
license_file "COPYING.LESSER"

# version_list: url=https://ftp.gnu.org/gnu/mpc/ filter=*.tar.gz

version("1.2.1") { source sha256: "17503d2c395dfcf106b622dc142683c1199431d095367c6aacba6eec30340459" }
version("1.1.0") { source sha256: "6985c538143c1208dcb1ac42cedad6ff52e267b47e5f970183a3e75125b43c2e" }

source url: "https://ftp.gnu.org/gnu/mpc/mpc-#{version}.tar.gz"

relative_path "mpc-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_command = ["./configure",
                       "--prefix=#{install_dir}/embedded"]

  command configure_command.join(" "), env: env
  make "-j #{workers}", env: env
  make "-j #{workers} install", env: env
end
