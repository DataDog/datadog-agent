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

name "mpfr"
default_version "4.1.0"

dependency "gmp"

license "LGPL-3.0-or-later"
license_file "COPYING.LESSER"

# version_list: url=https://ftp.gnu.org/gnu/mpfr/ filter=*.tar.gz

version("4.1.0") { source sha256: "3127fe813218f3a1f0adf4e8899de23df33b4cf4b4b3831a5314f78e65ffa2d6" }
version("3.1.6") { source sha256: "569ceb418aa935317a79e93b87eeb3f956cab1a97dfb2f3b5fd8ac2501011d62" }
version("3.1.3") { source sha256: "b87feae279e6da95a0b45eabdb04f3a35422dab0d30113d58a7803c0d73a79dc" }
version("3.1.2") { source sha256: "176043ec07f55cd02e91ee3219db141d87807b322179388413a9523292d2ee85" }

source url: "https://ftp.gnu.org/gnu/mpfr/mpfr-#{version}.tar.gz"

relative_path "mpfr-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_command = ["./configure",
                       "--prefix=#{install_dir}/embedded"]

  command configure_command.join(" "), env: env
  make "-j #{workers}", env: env
  make "-j #{workers} install", env: env
end
