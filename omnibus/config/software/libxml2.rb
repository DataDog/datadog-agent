#
# Copyright:: Chef Software Inc.
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

name "libxml2"
default_version "2.14.5"

license "MIT"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "zlib"
dependency "liblzma"
dependency "config_guess"

# version_list: url=https://download.gnome.org/sources/libxml2/2.14/ filter=*.tar.xz
version("2.14.5") { source sha256: "03d006f3537616833c16c53addcdc32a0eb20e55443cba4038307e3fa7d8d44b" }

source url: "https://download.gnome.org/sources/libxml2/2.14/libxml2-#{version}.tar.xz"

relative_path "libxml2-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_command = [
    "--with-zlib=#{install_dir}/embedded",
    "--with-lzma=#{install_dir}/embedded",
    "--with-sax1", # required for nokogiri to compile
    "--without-iconv",
    "--without-python",
    "--without-icu",
    "--without-debug",
    "--without-mem-debug",
    "--without-run-debug",
    "--without-legacy", # we don't need legacy interfaces
    "--without-catalog",
    "--without-docbook",
    "--disable-static",
  ]

  update_config_guess

  configure(*configure_command, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
