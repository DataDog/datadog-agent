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

name "libtool"
default_version "2.4.7"

license "GPL-2.0"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "config_guess"

# version_list: url=https://ftp.gnu.org/gnu/libtool/ filter=*.tar.gz

version("2.4.7") { source sha256: "04e96c2404ea70c590c546eba4202a4e12722c640016c12b9b2f1ce3d481e9a8" }

source url: "https://ftp.gnu.org/gnu/libtool/libtool-#{version}.tar.gz"

relative_path "libtool-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  update_config_guess
  update_config_guess(target: "libltdl/config")

  if aix?
    env["M4"] = "/opt/freeware/bin/m4"
  elsif solaris2?
    # We hit this bug on Solaris11 platforms bug#14291: libtool 2.4.2 fails to build due to macro_revision  reversion
    # The problem occurs with LANG=en_US.UTF-8 but not with LANG=C
    env["LANG"] = "C"
  end

  command "./configure" \
          " --disable-static" \
          " --prefix=#{install_dir}/embedded", env: env

  make env: env
  make "install", env: env
end
