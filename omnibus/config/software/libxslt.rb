#
# Copyright 2012-2014 Chef Software, Inc.
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

name "libxslt"
default_version "1.1.37"

license "MIT"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "libxml2"
dependency "liblzma"
dependency "config_guess"

# versions_list: url=https://download.gnome.org/sources/libxslt/1.1/ filter=*.tar.xz
version("1.1.37") { source sha256: "3a4b27dc8027ccd6146725950336f1ec520928f320f144eb5fa7990ae6123ab4" }

source url: "https://download.gnome.org/sources/libxslt/1.1/libxslt-#{version}.tar.xz"

relative_path "libxslt-#{version}"

build do
  update_config_guess

  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "libxslt-solaris-configure.patch", env: env if solaris2? || omnios? || smartos?

  if windows?
    patch source: "libxslt-windows-relocate.patch", env: env
  end

  # the libxslt configure script iterates directories specified in
  # --with-libxml-prefix looking for the libxml2 config script. That
  # iteration treats colons as a delimiter so we are using a cygwin
  # style path to accomodate
  configure_commands = [
    "--with-libxml-prefix=#{install_dir.sub("C:", "/C")}/embedded",
    "--without-python",
    "--without-crypto",
    "--without-profiler",
    "--without-debugger",
    "--disable-static",
  ]

  configure(*configure_commands, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
