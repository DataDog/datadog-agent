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

name "glib"
default_version "2.78.0"

license "LGPL-2.1"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "libffi"
dependency "pcre"
dependency "elfutils"

version("2.78.0") { source sha256: "a12ecee4622bc193bf32d683101ac486c74f1918abeb25ed0c8f644eedc5b5d4" }

ship_source_offer true

source url: "https://gitlab.gnome.org/GNOME/glib/-/archive/#{version}/glib-#{version}.tar.bz2"

relative_path "glib-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["LDFLAGS"] << " -Wl,--no-as-needed -ldl"

  patch source: "0001-Set-dependency-method-to-pkg-config.patch", env: env
  patch source: "0002-Disable-build-tests.patch", env: env

  meson_command = [
    "meson",
    "_build",
    "--prefix=#{install_dir}/embedded",
    "--libdir=lib",
    "-Dlibmount=disabled",
    "-Dselinux=disabled",
    "-Ddefault_library=static"
  ]

  command meson_command.join(" "), env: env

  command "ninja -C _build", env: env
  command "ninja -C _build install", env: env
end
