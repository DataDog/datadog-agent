#
# Copyright:: Copyright (c) 2014 Chef, Inc.
# License:: Apache License, Version 2.0
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

name "makedepend"
default_version "1.0.5"

source url: "https://xorg.freedesktop.org/releases/individual/util/makedepend-1.0.5.tar.gz",
       sha256: "503903d41fb5badb73cb70d7b3740c8b30fe1cc68c504d3b6a85e6644c4e5004",
       extract: :seven_zip

relative_path "makedepend-1.0.5"

dependency "xproto"
dependency "util-macros"

configure_env = with_standard_compiler_flags(with_embedded_path)

configure_env["PKG_CONFIG_LIBDIR"] = "#{install_dir}/embedded/lib/pkgconfig" +
  File::PATH_SEPARATOR +
  "#{install_dir}/embedded/share/pkgconfig"

build do
  license "BSD-3-Clause"
  license_file "https://raw.githubusercontent.com/ioerror/makedepend/master/LICENSE"

  configure env: configure_env
  command "make -j #{workers}", env: configure_env
  command "make -j #{workers} install", env: configure_env
end
