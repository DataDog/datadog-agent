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

name "systemd"
default_version "253"

license "GPLv2"
license "LGPL-2.1"
license_file "LICENSE.GPL2"
license_file "LICENSE.LGPL2.1"
skip_transitive_dependency_licensing true

version("253") { source sha256: "acbd86d42ebc2b443722cb469ad215a140f504689c7a9133ecf91b235275a491" }

ship_source_offer true

source url: "https://github.com/systemd/systemd/archive/refs/tags/v#{version}.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  # We only need the headers for coreos/go-systemd, and building
  # libsystemd itself would be fairly complicated as our toolchain doesn't
  # default include `/usr/include` in its default include path, while systemd
  # definitely need files in /usr/include/sys to build.
  mkdir "#{install_dir}/embedded/include/systemd"
  copy "src/systemd/*.h", "#{install_dir}/embedded/include/systemd/"
end
